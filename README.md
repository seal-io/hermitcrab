# Hermit Crab 

> tl;dr: Available Terraform Provider network mirroring service.

[![](https://goreportcard.com/badge/github.com/seal-io/hermitcrab)](https://goreportcard.com/report/github.com/seal-io/hermitcrab)
[![](https://img.shields.io/github/actions/workflow/status/seal-io/hermitcrab/ci.yml?label=ci)](https://github.com/seal-io/hermitcrab/actions)
[![](https://img.shields.io/docker/image-size/sealio/hermitcrab/main?label=docker)](https://hub.docker.com/r/sealio/hermitcrab/tags)
[![](https://img.shields.io/github/v/tag/seal-io/hermitcrab?label=release)](https://github.com/seal-io/hermitcrab/releases) 
[![](https://img.shields.io/github/license/seal-io/hermitcrab?label=license)](https://github.com/seal-io/hermitcrab#license)

Hermit Crab provides a stable and reliable [Terraform Provider](https://registry.terraform.io/browse/providers) network mirror service. 

This tool is maintained by [Seal](https://github.com/seal-io).

```mermaid
sequenceDiagram
    actor tf  as terraform init
    participant hc as Hermit Crab
    participant tfreg as Terraform Registry
    participant stg as Provider Package Storage
    
    tf ->> hc: list available versions
    alt not found
        hc ->> tfreg: list available versions
        tfreg -->> hc: 
        hc ->> hc: save
    end
    hc -->> tf: {"versions":{"2.0.0":{},"2.0.1":{}}}
    
    tf ->> hc: list available installation packages
    alt not found
        par
            hc ->> tfreg: find darwin/amd64 provider package
            tfreg -->> hc: 
            hc ->> hc: save
        and
            hc ->> tfreg: find linux/amd64 provider package
            tfreg -->> hc: 
            hc ->> hc: save
        end
    end
    hc -->> tf: {"archives": {"darwin_amd64":{},"linux_amd64":{}}
    
    tf ->> hc: download platform provider package, like darwin/amd64
    alt not found
        par not downloading
            hc ->> stg: download
            stg -->> hc: 
            hc ->> hc: store 
        and downloading
            hc ->> hc: wait until downloading finished
        end
    end
    hc -->> tf: ***.zip

```

## Background

When we drive [Terraform](https://www.terraform.io/) at some automation scenarios, like CI, automatic deployment, etc., we need to download the Terraform Provider plugins from the Internet by `terraform init`. 

However, depending on [Terraform Provider Registry Protocol](https://developer.hashicorp.com/terraform/internals/provider-registry-protocol), we may download a plugin that is not cached from an unstable networking remote.

To migrate the effect of unstable networking, Terraform provides two ways to solve this: [Implied Mirroring](https://developer.hashicorp.com/terraform/cli/config/config-file#implied-local-mirror-directories) and [Network Mirroring](https://developer.hashicorp.com/terraform/cli/config/config-file#network_mirror).

As far as **Implied Mirroring** is concerned, it works well when Terraform Provider version matches the [Version Constraints](https://developer.hashicorp.com/terraform/language/expressions/version-constraints). However, this mode fails when the version is not cached in the local file directory.

```
╷
│ Error: Failed to query available provider packages
│
│ Could not retrieve the list of available versions for provider
```

What's even more troublesome is that if the version changes, we need to continuously maintain this local file directory.

**Network Mirroring**, different from **Implied Mirroring**, can maintain all versions(including the latest) at a nearby network and allows distributed Terraform working agents to share the same mirroring package.

## Usage

Hermit Crab implements the [Terraform Provider Network Mirror](https://developer.hashicorp.com/terraform/internals/provider-network-mirror-protocol) protocol, and acts as a mirroring service.

We can serve Hermit Crab by [Docker](https://www.docker.com/).

```shell
docker run -d --restart=always -p 80:80 -p 443:443 sealio/hermitcrab
```

Hermit Crab saves the mirroring packages in the `/var/run/hermitcrab` directory by default, and we persist the mirroring packages by mounting a host path or a [Docker Volume](https://docs.docker.com/storage/volumes/).

```shell
docker run -d --restart=always -p 80:80 -p 443:443 \
  -v /tmp/hermitcrab:/var/run/hermitcrab \
  sealio/hermitcrab
```

Hermit Crab manages the archives as the following layer structure, which is absolutely compatible with the output of [`terraform providers mirror`](https://developer.hashicorp.com/terraform/cli/commands/providers/mirror).

```
/var/run/hermitcrab/data/providers
├── /<HOSTNAME>
│   ├── /<NAMESPACE>
│   │   ├── /<TYPE>
│   │   │   ├── terraform-provider-<TYPE>_<VERSION>_<OS>_<ARCH>.zip
```

Hermit Crab can reuse the mirroring providers by `terraform providers mirror` as well.

```shell
terraform providers mirror /tmp/providers

docker run -d --restart=always -p 80:80 -p 443:443 \
  -v /tmp/providers:/usr/share/terraform/providers \
  sealio/hermitcrab
```

Terraform Provider Network Mirror protocol wants [HTTPS](https://en.wikipedia.org/wiki/HTTPS) access, Hermit Crab provides multiple ways to achieve this.

- Use the default self-signed certificate, no additional configuration is required.

  Since Terraform always verifies the certificate insecure or not, under this mode, we need to import the self-signed certificate into the trusted certificate store.

  ```shell
  # download the self-signed certificate
  echo quit | openssl s_client -showcerts -servername <YOUR_SERVER> -connect <YOUR_ADDRESS> 2>/dev/null | openssl x509 -outform PEM >server.pem
  ```  

- Using [ACME](https://en.wikipedia.org/wiki/Automatic_Certificate_Management_Environment) to [gain a trusted certificate](https://letsencrypt.org/docs/challenge-types/), need a domain name and a DNS configuration.

  ```shell
  docker run -d --restart=always -p 80:80 -p 443:443 \
    -e SERVER_TLS_AUTO_CERT_DOMAINS=<YOUR_DOMAIN_NAME> \
    sealio/hermitcrab
  ```

- Use a custom certificate.
  
  ```shell
  docker run -d --restart=always -p 80:80 -p 443:443 \
    -v /<YOUR_PRIVATE_KEY_FILE>:/etc/hermitcrab/ssl/key.pem \
    -v /<YOUR_CERT_FILE>:/etc/hermitcrab/ssl/cert.pem \
    -e SERVER_TLS_PRIVATE_KEY_FILE=/etc/hermitcrab/ssl/key.pem \
    -e SERVER_TLS_CERT_FILE=/etc/hermitcrab/ssl/cert.pem \
    sealio/hermitcrab
  ```

After setting up Hermit Crab, you can configure the [CLI Configuration](https://developer.hashicorp.com/terraform/cli/config/config-file) as below to use the mirroring service.

```hcl
provider_installation {
  network_mirror {
   url = "https://<ADDRESS>/v1/providers/"
  }
}
```

## Notice

Hermit Crab is not a [Terraform Registry](https://registry.terraform.io), although implementing these protocols is not difficult, there are many options that you can choose from, like [HashiCorp Terraform Enterprise](https://www.hashicorp.com/products/terraform/pricing/), [JFrog Artifactory](https://jfrog.com/help/r/jfrog-artifactory-documentation/terraform-registry), etc.

Hermit Crab cannot mirror [Terraform Module](https://developer.hashicorp.com/terraform/internals/module-registry-protocol), since obtaining Terraform modules is diverse, like [Git](https://developer.hashicorp.com/terraform/language/modules/sources#generic-git-repository), [HTTP URLs](https://developer.hashicorp.com/terraform/language/modules/sources#http-urls), [S3 Bucket](https://developer.hashicorp.com/terraform/language/modules/sources#gcs-bucket) and so on, it's hard to provide a unified way to mirror them.

Hermit Crab doesn't support rewriting the provider [hostname](https://developer.hashicorp.com/terraform/internals/provider-network-mirror-protocol#hostname), which is a rare case and may make template/module reusing difficult. One possible scenario is that there is a private Terraform Registry in your network, and you need to use the community template/module without any modification.

Hermit Crab automatically synchronizes the in-use versions per 30 minutes, if the information update occurs during sleep, we can manually trigger the synchronization by sending a `PUT` request to `/v1/providers/sync`.

Hermit Crab only performs a checksum verification on the downloaded archives. For archives that already exist in the implied or explicit directory, checksum verification is not performed.

Hermit Crab only allows downloading the archives whose name matches the [Terraform Release Rules](https://developer.hashicorp.com/terraform/registry/providers/publishing#manually-preparing-a-release), which means the archive name must be `terraform-provider-<TYPE>_<VERSION>_<OS>_<ARCH>.zip`.

# License

Copyright (c) 2023 [Seal, Inc.](https://seal.io)

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at [LICENSE](./LICENSE) file for details.

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
