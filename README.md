# juicesync

![build](https://github.com/juicedata/juicesync/workflows/build/badge.svg) ![release](https://github.com/juicedata/juicesync/workflows/release/badge.svg)

Juicesync is a tool to move your data in object storage between any clouds or regions, also support local file, sftp and HDFS.

juicesync is an alias to `juicefs sync`, please use [juicefs](https://github.com/juicedata/juicefs) instead.

# How it works?

Juicesync will scan all the keys from two object stores, and comparing them in ascending order to find out missing or outdated keys, then download them from the source and upload them to the destination in parallel.

# Install

## With Homebrew

```sh
brew install juicedata/tap/juicesync
```

## Download binary release

From [here](https://github.com/juicedata/juicesync/releases)

# Build from source

Juicesync requires Go 1.16+ to build:

	```
	go get github.com/juicedata/juicesync
	```

# Upgrade

* Use Homebrew to upgrade or
* Download a new version from [release page](https://github.com/juicedata/juicesync/releases)

# Usage

```
$ juicesync -h
NAME:
   juicesync - rsync for cloud storage

USAGE:
   juicesync [options] SRC DST
    SRC and DST should be [NAME://][ACCESS_KEY:SECRET_KEY@]BUCKET[.ENDPOINT][/PREFIX]

VERSION:
   v0.7.1-2-g819f9c0

COMMANDS:
   help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --start KEY, -s KEY        the first KEY to sync
   --end KEY, -e KEY          the last KEY to sync
   --threads value, -p value  number of concurrent threads (default: 10)
   --http-port PORT           HTTP PORT to listen to (default: 6070)
   --update, -u               update existing file if the source is newer (default: false)
   --force-update, -f         always update existing file (default: false)
   --perms                    preserve permissions (default: false)
   --dirs                     Sync directories or holders (default: false)
   --dry                      don't copy file (default: false)
   --delete-src, --deleteSrc  delete objects from source after synced (default: false)
   --delete-dst, --deleteDst  delete extraneous objects from destination (default: false)
   --exclude PATTERN          exclude keys containing PATTERN (POSIX regular expressions)
   --include PATTERN          only include keys containing PATTERN (POSIX regular expressions)
   --manager value            manager address
   --worker value             hosts (seperated by comma) to launch worker
   --bwlimit value            limit bandwidth in Mbps (default: unlimited) (default: 0)
   --no-https                 donot use HTTPS (default: false)
   --verbose, -v              turn on debug log (default: false)
   --quiet, -q                change log level to ERROR (default: false)
   --help, -h                 show help (default: false)
   --version, -V              print only the version (default: false)
```

SRC and DST must be an URI of the following object storage:

- file: local files
- sftp: FTP via SSH
- s3: Amazon S3
- hdfs: Hadoop File System (HDFS)
- gcs: Google Cloud Storage
- wasb: Windows Azure Blob Storage
- oss: Aliyun OSS
- cos: Tencent Cloud COS
- ks3: KSYun KS3
- ufile: UCloud UFile
- qingstor: Qingcloud QingStor
- bos: Baidu Cloud Object Storage
- jss: JCloud Object Storage
- qiniu: Qiniu
- b2: Backblaze B2
- space: Digital Ocean Space
- obs: Huawei Object Storage Service
- oos: CTYun OOS
- scw: Scaleway Object Storage
- minio: MinIO
- scs: Sina Cloud Storage
- wasabi: Wasabi object storage
- ibmcos: IBM Cloud object storage
- webdav: WebDAV
- tikv:  TiKV cluster
- redis: Redis cluster
- mem: In-memory object store

please check the full list: [supported object storage](https://github.com/juicedata/juicefs/blob/main/docs/en/how_to_setup_object_storage.md#supported-object-storage)

SRC and DST should be in the following format:

[NAME://][ACCESS_KEY:SECRET_KEY@]BUCKET[.ENDPOINT][/PREFIX]

Some examples:

- local/path
- user@host:port:path
- file:///Users/me/code/
- hdfs://hdfs@namenode1:9000,namenode2:9000/user/
- s3://my-bucket/
- s3://access-key:secret-key-id@my-bucket/prefix
- wasb://account-name:account-key@my-container/prefix
- gcs://my-bucket.us-west1.googleapi.com/
- oss://test
- cos://test-1234
- obs://my-bucket
- bos://my-bucket
- minio://myip:9000/bucket
- scs://access-key:secret-key-id@my-bucket.sinacloud.net/prefix\
- webdav://host:port/prefix
- tikv://host1:port,host2:port,host3:port/prefix
- redis://localhost/1
- mem://

Note:

- It's recommended to run juicesync in the target region to have better performance.
- Auto discover endpoint for bucket of S3, OSS, COS, OBS, BOS, `SRC` and `DST` can use format `NAME://[ACCESS_KEY:SECRET_KEY@]BUCKET[/PREFIX]` . `ACCESS_KEY` and `SECRET_KEY` can be provided by corresponding environment variables (see below).
- When you get "/" in `ACCESS_KEY` or `SECRET_KEY` strings,you need to replace "/" with "%2F".
- S3:
  * The access key and secret key for S3 could be provided by `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY`, or *IAM* role.
- Wasb(Windows Azure Storage Blob)
  * The account name and account key can be provided as [connection string](https://docs.microsoft.com/en-us/azure/storage/common/storage-configure-connection-string#configure-a-connection-string-for-an-azure-storage-account) by `AZURE_STORAGE_CONNECTION_STRING`.
- GCS: The machine should be authorized to access Google Cloud Storage.
- OSS:
  * The credential can be provided by environment variable `ALICLOUD_ACCESS_KEY_ID` and `ALICLOUD_ACCESS_KEY_SECRET` , RAM role, [EMR MetaService](https://help.aliyun.com/document_detail/43966.html).
- COS:
  * The AppID should be part of the bucket name.
  * The credential can be provided by environment variable `COS_SECRETID` and `COS_SECRETKEY`.
- OBS:
  * The credential can be provided by environment variable `HWCLOUD_ACCESS_KEY` and `HWCLOUD_SECRET_KEY` .
- BOS:
  * The credential can be provided by environment variable `BDCLOUD_ACCESS_KEY` and `BDCLOUD_SECRET_KEY` .
- Qiniu:
  The S3 endpoint should be used for Qiniu, for example, abc.cn-north-1-s3.qiniu.com.
  If there are keys starting with "/", the domain should be provided as `QINIU_DOMAIN`.
- sftp: if your target machine uses SSH certificates instead of password, you should pass the path to your private key file to the environment variable `SSH_PRIVATE_KEY_PATH`, like ` SSH_PRIVATE_KEY_PATH=/home/someuser/.ssh/id_rsa juicesync [src] [dst]`.
- Scaleway:
  * The credential can be provided by environment variable `SCW_ACCESS_KEY` and `SCW_SECRET_KEY` .
- MinIO:
  * The credential can be provided by environment variable `MINIO_ACCESS_KEY` and `MINIO_SECRET_KEY` .
