package download

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/seal-io/walrus/utils/bytespool"
	"github.com/seal-io/walrus/utils/gopool"
	"github.com/seal-io/walrus/utils/log"
	"github.com/seal-io/walrus/utils/runtimex"
	"github.com/seal-io/walrus/utils/version"
)

var defaultHttpClient = NewHttpClient(
	WithUserAgent(version.GetUserAgentWith("hermitcrab")),
	WithInsecureSkipVerify(),
)

type Client struct {
	httpCli *http.Client
}

func NewClient(httpCli *http.Client) *Client {
	if httpCli == nil {
		httpCli = defaultHttpClient
	}

	return &Client{
		httpCli: httpCli,
	}
}

type GetOptions struct {
	DownloadURL string
	Directory   string
	Filename    string
	Shasum      string
}

func (c *Client) Get(ctx context.Context, opts GetOptions) error {
	if opts.DownloadURL == "" || opts.Directory == "" || opts.Filename == "" {
		return errors.New("invalid options")
	}

	output := filepath.Join(opts.Directory, opts.Filename)

	// Validate the output,
	// if existed, return directly,
	// check corrupted if the shasum is provided.
	if info, err := os.Lstat(output); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("validate: failed to get output info: %w", err)
	} else if info != nil {
		// Validate if the output is a directory.
		if info.IsDir() {
			return errors.New("validate: output is a directory")
		}

		// Get real path if the output is a symlink.
		if info.Mode()&os.ModeSymlink != 0 {
			output, err = os.Readlink(output)
			if err != nil {
				return errors.New("validate: failed to get real output")
			}
		}

		// Validate the shasum.
		matched, err := validateShasum(output, opts.Shasum)
		if err != nil {
			return fmt.Errorf("validate: failed to validate existing output: %w", err)
		}

		// Return directly if the shasum is matched.
		if matched {
			return nil
		}

		// Remove the corrupted existing output.
		err = os.RemoveAll(output)
		if err != nil {
			return fmt.Errorf("validate: failed to remove corrupted existing output: %w", err)
		}
	}

	// Validate the temp output,
	// if existed, must check the shasum.
	var (
		tempPath       = filepath.Join(opts.Directory, "."+opts.Filename)
		receivedLength int64
	)
	{
		if info, err := os.Lstat(tempPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("validate: failed to get temp output info: %w", err)
		} else if info != nil {
			receivedLength = info.Size()

			// Correct the temp output if it is not a regular file.
			if !info.Mode().IsRegular() {
				err = os.RemoveAll(tempPath)
				if err != nil {
					return fmt.Errorf("validate: failed to remove corrupted temp output: %w", err)
				}
			}
		}
	}

	// Check if the remote allowing range download.
	var (
		partialDownload bool
		contentLength   int64
	)
	{
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, opts.DownloadURL, nil)
		if err != nil {
			return fmt.Errorf("download: failed to create HEAD request: %w", err)
		}

		resp, err := c.httpCli.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			partialDownload = resp.Header.Get("Accept-Ranges") == "bytes" &&
				resp.ContentLength > 0 &&
				runtimex.NumCPU() > 1
			contentLength = resp.ContentLength
		}

		// If the remote allowing range download,
		// but the temp output is larger than the target size,
		// we should remove the temp output and download again.
		if partialDownload && receivedLength > contentLength {
			err = os.RemoveAll(tempPath)
			if err != nil {
				return fmt.Errorf("download: failed to remove corrupted temp output: %w", err)
			}

			receivedLength = 0
		}
	}

	// Prepare the output directory.
	err := os.MkdirAll(opts.Directory, 0o700)
	if err != nil && !os.IsExist(err) {
		return fmt.Errorf("download: failed to create output directory: %w", err)
	}

	// Download.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, opts.DownloadURL, nil)
	if err != nil {
		return fmt.Errorf("download: failed to create GET request: %w", err)
	}

	tempFile, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("download: failed to open temp file: %w", err)
	}

	defer func() {
		_ = tempFile.Close()

		if err == nil || partialDownload {
			return
		}

		// Remove the temp file if failed to download.
		_ = os.Remove(tempPath)
	}()

	if partialDownload {
		err = c.downloadPartial(req, tempFile, receivedLength, contentLength)
	} else {
		err = c.download(req, tempFile)
	}

	if err != nil {
		return fmt.Errorf("download: %w", err)
	}

	// Validate whether the shasum is matched.
	matched, err := validateShasum(tempPath, opts.Shasum)
	if err != nil {
		return fmt.Errorf("validate: failed to validate downloaded temp output: %w", err)
	}

	if !matched {
		// Remove the corrupted download output.
		err = os.RemoveAll(tempPath)
		if err != nil {
			return fmt.Errorf("validate: failed to remove corrupted download output: %w", err)
		}

		return errors.New("validate: shasum mismatched")
	}

	err = os.Rename(tempPath, output)
	if err != nil {
		return fmt.Errorf("download: failed to rename output: %w", err)
	}

	return nil
}

func (c *Client) downloadPartial(req *http.Request, file *os.File, receivedLength, contentLength int64) error {
	if receivedLength == contentLength {
		return nil
	}

	logger := log.WithName("download").WithValues("url", req.URL)

	if receivedLength == 0 {
		err := file.Truncate(contentLength)
		if err != nil {
			return fmt.Errorf("failed to truncate file: %w", err)
		}
	} else {
		_, err := file.Seek(0, io.SeekEnd)
		if err != nil {
			return fmt.Errorf("failed to seek file to end: %w", err)
		}
	}

	const (
		partialBuffer = 2 * 1024 * 1024 // 2mb.
		parallel      = 5
	)

	var bytesRanges [][2]int64
	{
		for start := receivedLength; start < contentLength; {
			end := start + partialBuffer
			if end >= contentLength {
				end = contentLength
			}

			bytesRanges = append(bytesRanges, [2]int64{start, end})
			start = end
		}
	}

	logger.Debug("downloading")

	for i, t := 0, len(bytesRanges); i < t; {
		j := i + parallel
		if j >= t {
			j = t
		}

		err := func(bytesRanges [][2]int64) error {
			var (
				partialStart = bytesRanges[0][0]
				partialEnd   = bytesRanges[len(bytesRanges)-1][1]
				buf          = make([]byte, partialEnd-partialStart)
			)

			wg := gopool.GroupWithContextIn(req.Context())

			for k := range bytesRanges {
				var (
					rangeStart = bytesRanges[k][0]
					rangeEnd   = bytesRanges[k][1]
				)

				wg.Go(func(ctx context.Context) error {
					req := req.Clone(ctx)
					req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", rangeStart, rangeEnd))

					resp, err := c.httpCli.Do(req)
					if err != nil {
						return fmt.Errorf("failed to send partital GET request: %w", err)
					}

					defer func() { _ = resp.Body.Close() }()

					if resp.StatusCode != http.StatusPartialContent {
						return fmt.Errorf("unexpected partital GET response status: %s", resp.Status)
					}

					var (
						bufStart = rangeStart - partialStart
						bufEnd   = rangeEnd - partialStart
					)

					_, err = io.ReadFull(resp.Body, buf[bufStart:bufEnd])
					if err != nil {
						return err
					}

					logger.V(6).Infof("received range %d-%d", rangeStart, rangeEnd)

					return nil
				})
			}

			err := wg.Wait()
			if err != nil {
				return err
			}

			_, err = file.Write(buf)
			if err != nil {
				return fmt.Errorf("failed to output partital response body %d-%d: %w",
					partialStart, partialEnd, err)
			}

			return nil
		}(bytesRanges[i:j])
		if err != nil {
			return err
		}

		i = j
	}

	logger.Debug("downloaded")

	return nil
}

const copyBuffer = 1024 * 1024 // 1mb.

func (c *Client) download(req *http.Request, file *os.File) error {
	logger := log.WithName("download").WithValues("url", req.URL)

	// Seek to the beginning of the temp file.
	_, err := file.Seek(0, 0)
	if err != nil {
		return fmt.Errorf("failed to seek file beginning: %w", err)
	}

	logger.Debug("downloading")

	resp, err := c.httpCli.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send GET request: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	// Validate the response.
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected GET response status: %s", resp.Status)
	}

	buf := bytespool.GetBytes(copyBuffer)
	defer bytespool.Put(buf)

	// Write the response body to the temp file.
	_, err = io.CopyBuffer(file, resp.Body, buf)
	if err != nil {
		return fmt.Errorf("failed to output response body: %w", err)
	}

	logger.Debug("downloaded")

	return nil
}

func validateShasum(path, shasum string) (bool, error) {
	if shasum == "" {
		return true, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return false, err
	}

	defer func() { _ = f.Close() }()

	h := sha256.New()

	buf := bytespool.GetBytes(copyBuffer)
	defer bytespool.Put(buf)

	_, err = io.CopyBuffer(h, f, buf)
	if err != nil {
		return false, err
	}

	return hex.EncodeToString(h.Sum(nil)) == shasum, nil
}
