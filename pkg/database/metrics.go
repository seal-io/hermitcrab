package database

import (
	"bytes"
	"path"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	bolt "go.etcd.io/bbolt"
)

const (
	namespace   = "boltdb"
	txSubsystem = "tx"
	bkSubsystem = "bk"
)

func NewStatsCollectorWith(db BoltDriver) prometheus.Collector {
	return &statsCollector{
		d: NewDatabaseStatsCollectorWith(db),
		b: NewBucketStatsCollector(db),
	}
}

type statsCollector struct {
	sync.Mutex

	d, b prometheus.Collector
}

func (c *statsCollector) Describe(ch chan<- *prometheus.Desc) {
	c.Lock()
	defer c.Unlock()

	c.d.Describe(ch)
	c.b.Describe(ch)
}

func (c *statsCollector) Collect(ch chan<- prometheus.Metric) {
	c.Lock()
	defer c.Unlock()

	c.d.Collect(ch)
	c.b.Collect(ch)
}

func NewDatabaseStatsCollectorWith(db BoltDriver) prometheus.Collector {
	return &databaseStatsCollector{
		db: db,
		freelistFreePages: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "freelist_free_pages"),
			"The number of free pages in the database.",
			nil, nil,
		),
		freelistPendingPages: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "freelist_pending_pages"),
			"The number of pending pages in the database.",
			nil, nil,
		),
		freelistAllocatedBytes: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "freelist_allocated_bytes"),
			"The number of allocated bytes in the database.",
			nil, nil,
		),
		freelistInUseBytes: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "freelist_in_use_bytes"),
			"The number of in-use bytes in the database.",
			nil, nil,
		),
		txReads: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, txSubsystem, "reads_total"),
			"The total number of database reads.",
			nil, nil,
		),
		txReadsOpen: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, txSubsystem, "reads_open"),
			"The number of database reads currently open.",
			nil, nil,
		),
		txPagesAllocated: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, txSubsystem, "pages_allocated_total"),
			"The total number of pages allocated.",
			nil, nil,
		),
		txPagesAllocatedBytes: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, txSubsystem, "pages_allocated_bytes_total"),
			"The total number of bytes allocated.",
			nil, nil,
		),
		txCursors: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, txSubsystem, "cursors_total"),
			"The total number of cursors created.",
			nil, nil,
		),
		txNodesAllocated: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, txSubsystem, "nodes_allocated_total"),
			"The total number of nodes allocated.",
			nil, nil,
		),
		txNodesDereference: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, txSubsystem, "nodes_dereference_total"),
			"The total number of nodes dereference.",
			nil, nil,
		),
		txNodeRebalanced: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, txSubsystem, "node_rebalanced_total"),
			"The total number of node rebalanced.",
			nil, nil,
		),
		txNodeRebalancedSeconds: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, txSubsystem, "node_rebalanced_seconds_total"),
			"The total number of node rebalanced seconds.",
			nil, nil,
		),
		txNodesSplit: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, txSubsystem, "nodes_split_total"),
			"The total number of nodes split.",
			nil, nil,
		),
		txNodesSpilt: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, txSubsystem, "nodes_spilt_total"),
			"The total number of nodes spilt.",
			nil, nil,
		),
		txNodesSpiltSeconds: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, txSubsystem, "nodes_spilt_seconds_total"),
			"The total number of nodes spilt seconds.",
			nil, nil,
		),
		txWrites: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, txSubsystem, "writes_total"),
			"The total number of database writes.",
			nil, nil,
		),
		txWriteSeconds: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, txSubsystem, "write_seconds_total"),
			"The total number of database write seconds.",
			nil, nil,
		),
	}
}

type databaseStatsCollector struct {
	db BoltDriver

	freelistFreePages      *prometheus.Desc
	freelistPendingPages   *prometheus.Desc
	freelistAllocatedBytes *prometheus.Desc
	freelistInUseBytes     *prometheus.Desc

	txReads                 *prometheus.Desc
	txReadsOpen             *prometheus.Desc
	txPagesAllocated        *prometheus.Desc
	txPagesAllocatedBytes   *prometheus.Desc
	txCursors               *prometheus.Desc
	txNodesAllocated        *prometheus.Desc
	txNodesDereference      *prometheus.Desc
	txNodeRebalanced        *prometheus.Desc
	txNodeRebalancedSeconds *prometheus.Desc
	txNodesSplit            *prometheus.Desc
	txNodesSpilt            *prometheus.Desc
	txNodesSpiltSeconds     *prometheus.Desc
	txWrites                *prometheus.Desc
	txWriteSeconds          *prometheus.Desc
}

func (c *databaseStatsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.freelistFreePages
	ch <- c.freelistPendingPages
	ch <- c.freelistAllocatedBytes
	ch <- c.freelistInUseBytes

	ch <- c.txReadsOpen
	ch <- c.txReads
	ch <- c.txPagesAllocated
	ch <- c.txPagesAllocatedBytes
	ch <- c.txCursors
	ch <- c.txNodesAllocated
	ch <- c.txNodesDereference
	ch <- c.txNodeRebalanced
	ch <- c.txNodeRebalancedSeconds
	ch <- c.txNodesSplit
	ch <- c.txNodesSpilt
	ch <- c.txNodesSpiltSeconds
	ch <- c.txWrites
	ch <- c.txWriteSeconds
}

func (c *databaseStatsCollector) Collect(ch chan<- prometheus.Metric) {
	stats := c.db.Stats()
	txStats := stats.TxStats

	ch <- prometheus.MustNewConstMetric(c.freelistFreePages, prometheus.GaugeValue, float64(stats.FreePageN))
	ch <- prometheus.MustNewConstMetric(c.freelistPendingPages, prometheus.GaugeValue, float64(stats.PendingPageN))
	ch <- prometheus.MustNewConstMetric(c.freelistAllocatedBytes, prometheus.GaugeValue, float64(stats.FreeAlloc))
	ch <- prometheus.MustNewConstMetric(c.freelistInUseBytes, prometheus.GaugeValue, float64(stats.FreelistInuse))

	ch <- prometheus.MustNewConstMetric(c.txReadsOpen, prometheus.GaugeValue, float64(stats.OpenTxN))
	ch <- prometheus.MustNewConstMetric(c.txReads, prometheus.CounterValue, float64(stats.TxN))
	ch <- prometheus.MustNewConstMetric(c.txPagesAllocated, prometheus.CounterValue, float64(txStats.GetPageCount()))
	ch <- prometheus.MustNewConstMetric(c.txPagesAllocatedBytes, prometheus.CounterValue, float64(txStats.GetPageAlloc()))
	ch <- prometheus.MustNewConstMetric(c.txCursors, prometheus.CounterValue, float64(txStats.GetCursorCount()))
	ch <- prometheus.MustNewConstMetric(c.txNodesAllocated, prometheus.CounterValue, float64(txStats.GetNodeCount()))
	ch <- prometheus.MustNewConstMetric(c.txNodesDereference, prometheus.CounterValue, float64(txStats.GetNodeDeref()))
	ch <- prometheus.MustNewConstMetric(c.txNodeRebalanced, prometheus.CounterValue, float64(txStats.GetRebalance()))
	ch <- prometheus.MustNewConstMetric(c.txNodeRebalancedSeconds, prometheus.CounterValue, txStats.GetRebalanceTime().Seconds())
	ch <- prometheus.MustNewConstMetric(c.txNodesSplit, prometheus.CounterValue, float64(txStats.GetSplit()))
	ch <- prometheus.MustNewConstMetric(c.txNodesSpilt, prometheus.CounterValue, float64(txStats.GetSpill()))
	ch <- prometheus.MustNewConstMetric(c.txNodesSpiltSeconds, prometheus.CounterValue, txStats.GetSpillTime().Seconds())
	ch <- prometheus.MustNewConstMetric(c.txWrites, prometheus.CounterValue, float64(txStats.GetWrite()))
	ch <- prometheus.MustNewConstMetric(c.txWriteSeconds, prometheus.CounterValue, txStats.GetWriteTime().Seconds())
}

func NewBucketStatsCollector(db BoltDriver) prometheus.Collector {
	labels := []string{"bucket"}

	return &bucketStatsCollector{
		db: db,
		depth: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, bkSubsystem, "depth"),
			"The depth of the bucket.",
			labels, nil,
		),
		keys: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, bkSubsystem, "keys"),
			"The number of keys in the bucket.",
			labels, nil,
		),
		buckets: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, bkSubsystem, "buckets"),
			"The number of buckets in the bucket.",
			labels, nil,
		),
		inlinedBuckets: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, bkSubsystem, "inlined_buckets"),
			"The number of inlined buckets in the bucket.",
			labels, nil,
		),
		inlinedBucketsInUseBytes: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, bkSubsystem, "inlined_buckets_in_use_bytes"),
			"The number of in-use bytes in the bucket.",
			labels, nil,
		),
		logicalLeafPages: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, bkSubsystem, "logical_leaf_pages"),
			"The number of logical leaf pages in the bucket.",
			labels, nil,
		),
		logicalBranchPages: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, bkSubsystem, "logical_branch_pages"),
			"The number of logical branch pages in the bucket.",
			labels, nil,
		),
		physicalLeafOverflowPages: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, bkSubsystem, "physical_leaf_overflow_pages"),
			"The number of physical leaf overflow pages in the bucket.",
			labels, nil,
		),
		physicalLeafPagesAllocatedBytes: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, bkSubsystem, "physical_leaf_pages_allocated_bytes"),
			"The number of physical leaf pages allocated bytes in the bucket.",
			labels, nil,
		),
		physicalLeafPagesInUseBytes: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, bkSubsystem, "physical_leaf_pages_in_use_bytes"),
			"The number of physical leaf pages in use bytes in the bucket.",
			labels, nil,
		),
		physicalBranchOverflowPages: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, bkSubsystem, "physical_branch_overflow_pages"),
			"The number of physical branch overflow pages in the bucket.",
			labels, nil,
		),
		physicalBranchPagesAllocatedBytes: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, bkSubsystem, "physical_branch_pages_allocated_bytes"),
			"The number of physical branch pages allocated bytes in the bucket.",
			labels, nil,
		),
		physicalBranchPagesInUseBytes: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, bkSubsystem, "physical_branch_pages_in_use_bytes"),
			"The number of physical branch pages in use bytes in the bucket.",
			labels, nil,
		),
	}
}

type bucketStatsCollector struct {
	db BoltDriver

	depth                             *prometheus.Desc
	keys                              *prometheus.Desc
	buckets                           *prometheus.Desc
	inlinedBuckets                    *prometheus.Desc
	inlinedBucketsInUseBytes          *prometheus.Desc
	logicalLeafPages                  *prometheus.Desc
	logicalBranchPages                *prometheus.Desc
	physicalLeafOverflowPages         *prometheus.Desc
	physicalLeafPagesAllocatedBytes   *prometheus.Desc
	physicalLeafPagesInUseBytes       *prometheus.Desc
	physicalBranchOverflowPages       *prometheus.Desc
	physicalBranchPagesAllocatedBytes *prometheus.Desc
	physicalBranchPagesInUseBytes     *prometheus.Desc
}

func (c *bucketStatsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.depth
	ch <- c.keys
	ch <- c.buckets
	ch <- c.inlinedBuckets
	ch <- c.inlinedBucketsInUseBytes
	ch <- c.logicalLeafPages
	ch <- c.logicalBranchPages
	ch <- c.physicalLeafOverflowPages
	ch <- c.physicalLeafPagesAllocatedBytes
	ch <- c.physicalLeafPagesInUseBytes
	ch <- c.physicalBranchOverflowPages
	ch <- c.physicalBranchPagesAllocatedBytes
	ch <- c.physicalBranchPagesInUseBytes
}

func (c *bucketStatsCollector) Collect(ch chan<- prometheus.Metric) {
	err := c.db.View(func(tx *bolt.Tx) error {
		return tx.ForEach(func(k []byte, b *bolt.Bucket) error {
			return c.collect(ch, string(bytes.Clone(k)), b)
		})
	})
	if err != nil {
		ch <- prometheus.NewInvalidMetric(c.buckets, err)
	}
}

func (c *bucketStatsCollector) collect(ch chan<- prometheus.Metric, n string, b *bolt.Bucket) error {
	stats := b.Stats()

	ch <- prometheus.MustNewConstMetric(c.depth, prometheus.GaugeValue,
		float64(stats.Depth), n)
	ch <- prometheus.MustNewConstMetric(c.keys, prometheus.GaugeValue,
		float64(stats.KeyN), n)
	ch <- prometheus.MustNewConstMetric(c.buckets, prometheus.GaugeValue,
		float64(stats.BucketN), n)
	ch <- prometheus.MustNewConstMetric(c.inlinedBuckets, prometheus.GaugeValue,
		float64(stats.InlineBucketN), n)
	ch <- prometheus.MustNewConstMetric(c.inlinedBucketsInUseBytes, prometheus.GaugeValue,
		float64(stats.InlineBucketInuse), n)
	ch <- prometheus.MustNewConstMetric(c.logicalLeafPages, prometheus.GaugeValue,
		float64(stats.LeafPageN), n)
	ch <- prometheus.MustNewConstMetric(c.logicalBranchPages, prometheus.GaugeValue,
		float64(stats.BranchPageN), n)
	ch <- prometheus.MustNewConstMetric(c.physicalLeafOverflowPages, prometheus.GaugeValue,
		float64(stats.LeafOverflowN), n)
	ch <- prometheus.MustNewConstMetric(c.physicalLeafPagesAllocatedBytes, prometheus.GaugeValue,
		float64(stats.LeafAlloc), n)
	ch <- prometheus.MustNewConstMetric(c.physicalLeafPagesInUseBytes, prometheus.GaugeValue,
		float64(stats.LeafInuse), n)
	ch <- prometheus.MustNewConstMetric(c.physicalBranchOverflowPages, prometheus.GaugeValue,
		float64(stats.BranchOverflowN), n)
	ch <- prometheus.MustNewConstMetric(c.physicalBranchPagesAllocatedBytes, prometheus.GaugeValue,
		float64(stats.BranchAlloc), n)
	ch <- prometheus.MustNewConstMetric(c.physicalBranchPagesInUseBytes, prometheus.GaugeValue,
		float64(stats.BranchInuse), n)

	return b.ForEachBucket(func(k []byte) error {
		return c.collect(ch, path.Join(n, string(bytes.Clone(k))), b.Bucket(k))
	})
}
