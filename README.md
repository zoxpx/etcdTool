# etcdTool

etcdTool is a dump/restore tool for etcd3.

This tool works with the etcd3 databases, and provides a basic list/get/put/remove functionality similar to etcdctl utility that comes with etcd3.

However, unlike the etcdctl it can dump the database to the file-system, or into the ZIP, TAR or TAR-GZ archives.   Please note that this is done _without_ any kind of database locks, so we make no guarantees about database consistency.


## General syntax

	NAME:
	   etcdTool - A dump/restore tool for etcd3.
	
	USAGE:
	   etcdTool <list|get|put|remove|dump|upload|tar|zip> [command options] [arguments...]
	
	ENVIRONMENT VARIABLES:
	   ETCD_LISTEN_CLIENT_URLS      Changes default endpoint
	
	VERSION:
	   1.2
	
	COMMANDS:
	     list, ls    list keys
	     get         get keys
	     put         put key
	     remove, rm  remove keys
	     dump        dump keys
	     upload, up  upload keys
	     tar         create TAR archive from the EtcD keys
	     zip         create ZIP archive from the EtcD keys
	     help, h     Shows a list of commands or help for one command
	
	GLOBAL OPTIONS:
	   --endpoints value, -e value  Specify endpoints (default: "127.0.0.1:2379")
	   --timeout value, -T value    Specify timeout (default: 5)
	   --debug                      Turn on debug output
	   --help, -h                   show help
	   --version, -v                print the version

## Basic CRUD operations

### LIST keys

	NAME:
	   etcdTool list - list keys
	
	USAGE:
	   etcdTool list [arguments...]

The `list` command will display the keys in the etcd3.  If no argument is given, the whole etcd3 database will be listed.
If we did provide an argument, only the keys with that prefix will be listed.

### PUT key

	NAME:
	   etcdTool put - put key
	
	USAGE:
	   etcdTool put <file|-> key
	
	OPTIONS:
	   --e64  perform base64 encoding

The `put` command inserts a file into the given etcd3 key.  If `-` was provided instead of a file, the input will be read from the STDIN.

> **NOTE**:<br/> The etcd3 cannot store the binary content.  Therefore, the `put` command also supports `--e64` option, which will perform a [base64](https://en.wikipedia.org/wiki/Base64) encoding on the content before storing.

### GET key

	NAME:
	   etcdTool get - get keys
	
	USAGE:
	   etcdTool get key1 [key2...]
	
	OPTIONS:
	   --d64  perform base64 decoding

The `get` command retrieves the given content out of the etcd3 database.  The key's data (the values) will be displayed directly on the STDOUT.

> **NOTE**:<br/> The etcd3 cannot store the binary content.  If the content was stored using [base64](https://en.wikipedia.org/wiki/Base64) encoding, you can use the `--d64` option to decode the content back into binary, before displaying it on the screen.

### REMOVE key

	NAME:
	   etcdTool remove - remove keys
	
	USAGE:
	   etcdTool rm key1 [key2/ ...]
	
	DESCRIPTION:
	   Remove command removes keys or directories from the EtcD.
	   If a key-parameter ends with '/' (e.g. key/), the key will be interpreted as a directory,
	   and everything inside this directory will be removed.

The `remove` (`rm`) command removes the keys from the etcd3.  Removing the keys ending with `/` (e.g. `foo/`) will trigged *recursive removal* of the content.

> <span style="color:red">**WARNING**</span>:<br/> Please exercise caution when removing content from the etcd3 database.  Once removed, the content cannot be retrieved, unless you can perform a restore from a recent database snapshot, or have a content-dump.<br/>
> This is especially important with *recursive deletions*, triggered by removing keys ending with "/".

## Dump/Upload operations

### DUMP keys

	NAME:
	   etcdTool dump - dump keys
	
	USAGE:
	   etcdTool dump [-C <dir>] key1 [key2...]
	
	OPTIONS:
	   --directory value, -C value  save keys into directory
	   --d64                        perform base64 decoding
	   --strip                      strip path of the key

The `dump` command will download the etcd3 content to a local file-system.

### UPLOAD keys

	NAME:
	   etcdTool upload - upload keys
	
	USAGE:
	   etcdTool upload [-C dir] dir1 [dir2...]
	
	OPTIONS:
	   --directory value, -C value  load keys from directory
	   --e64                        perform base64 encoding
	   --prefix value               prefix the keys on upload

The `upload` command can take a directory's content, and upload files as keys into etcd3.

In conjunction with `dump` command, it can be used as a powerful tool to "dump-modify-upload" big amount of etcd3 keys, or to create copies of keys/directories under a different path.

## TAR/ZIP operations

### TAR

	NAME:
	   etcdTool tar - create TAR archive from the EtcD keys
	
	USAGE:
	   etcdTool tar [-f <file.tar>] [-z] key1 [key2...]
	
	OPTIONS:
	   -f value  specify TAR filename
	   -z        compress archive (GZip)

The `tar` command downloads the etcd3 content into [TAR](https://en.wikipedia.org/wiki/Tar) (or TAR-GZ) archive.
Please note that similar like [tar(1)](https://linux.die.net/man/1/tar), the output will by default go to the STDOUT, unless redirected into a file via `-f file` option.

### ZIP

	NAME:
	   etcdTool zip - create ZIP archive from the EtcD keys
	
	USAGE:
	   etcdTool zip -f <file.tar> key1 [key2...]
	
	OPTIONS:
	   -f value  specify ZIP filename

The `zip` command downloads the etcd3 content into [ZIP](https://en.wikipedia.org/wiki/Zip) archive.

## Known Limitations

* content locking while keys are being uploaded/downloaded
	- **CAVEAT**: in concurrent-access scenarios you might be downloading corrupted/incomplete data
* no authentication support  (i.e. cannot work with user/password -protected etcd3)
* no client SSL support  (i.e. does not work with client-side SSL certificates)

## Legal

**NO WARRANTY.**  THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.