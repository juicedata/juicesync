# juicesync

![build](https://github.com/juicedata/juicesync/workflows/build/badge.svg) ![release](https://github.com/juicedata/juicesync/workflows/release/badge.svg)

`juicesync` is a tool to copy your data in object storage between any clouds or regions, it also supports local disk, SFTP, HDFS and many more.

This tool shares code with [`juicefs sync`](https://github.com/juicedata/juicefs), so if you are already using [JuiceFS Community Edition or Cloud Service](https://juicefs.com/en/), you should use `juicefs sync` instead.

Due to release planning, `juicesync` may not contain the latest features and bug fixes of `juicefs sync`.

## How does it work

`juicesync` will scan all the keys from two storage systems, and comparing them in ascending order to find out missing or outdated keys, then download them from the source and upload them to the destination in parallel.

## Install

`juicesync` is an alias for the `juicefs sync` command, you're encouraged to install the JuiceFS Client and run `juicefs sync` instead:

* [Install JuiceFS Community Edition](https://juicefs.com/docs/community/installation/)
* [Install JuiceFS Cloud Service](https://juicefs.com/docs/cloud/getting_started/#installation)

Above versions are more actively maintained and contain the latest improvements and bug fixes, however, if you'd like to use solely the `sync` functionality, you can install the standalone `juicesync` tool.

* Download `juicesync` binary release from [here](https://github.com/juicedata/juicesync/releases)
* Download this repo and build from source (requires Go 1.16+ to build): `go get github.com/juicedata/juicesync`

## Usage

Since `juicesync` is just an alias for the `juicefs sync` command, refer to JuiceFS documentation for detailed usage:


* [JuiceFS Community Edition](https://juicefs.com/docs/community/guide/sync/)
* [JuiceFS Cloud Service](https://juicefs.com/docs/cloud/guide/sync/)
