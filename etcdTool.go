package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/coreos/etcd/mvcc/mvccpb"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
	"go.etcd.io/etcd/clientv3"
)

const (
	version              = "1.6"
	unicodeFractSlashStr = "\u2044" // reserved unicode char
)

var (
	ctx = context.Background()
	opt = struct {
		endpoints string
		timeout   int
	}{
		endpoints: "127.0.0.1:2379",
		timeout:   5,
	}
	unicodeFractSlashBytes = []byte(unicodeFractSlashStr)
)

// kvKey2FileName is a WORKAROUND transformation function - will convert `xxx/` keys into `xxx\u2044` file-names
func kvKey2FileName(kv *mvccpb.KeyValue) string {
	if kv == nil || len(kv.Key) <= 0 {
		logrus.Fatal("Invalid key name")
	}
	ky := kv.Key
	if ll := len(ky); ky[ll-1] == '/' {
		ky = append(ky[:ll-1], unicodeFractSlashBytes...)
	}
	return string(ky)
}

// fileName2KvKey is a WORKAROUND transformation function - will convert `xxx\u2044` file-names into `xxx/` keys
func fileName2KvKey(in string) string {
	if in == "" {
		logrus.Fatal("Invalid file name")
	}
	if strings.HasSuffix(in, unicodeFractSlashStr) {
		return in[:len(in)-len(unicodeFractSlashStr)] + "/"
	}
	return in
}

func getEtcdClient() *clientv3.Client {
	client, err := clientv3.New(clientv3.Config{
		Endpoints:            strings.Split(opt.endpoints, ","),
		DialTimeout:          time.Duration(opt.timeout) * time.Second,
		DialKeepAliveTime:    time.Duration(opt.timeout) * time.Second,
		DialKeepAliveTimeout: time.Duration(opt.timeout) * time.Second * 3,
	})
	if err != nil {
		logrus.WithError(err).Panicf("clientv3.New() failed")
	}
	return client
}

func checkErr(err error) {
	if err != nil {
		logrus.Fatal(err)
		os.Exit(-1)
	}
}

func countKeys(path string) int64 {
	var (
		client = getEtcdClient()
		opts   = []clientv3.OpOption{
			clientv3.WithPrefix(),
			clientv3.WithCountOnly(),
		}
	)

	res, err := client.Get(ctx, path, opts...)
	checkErr(err)
	return res.Count
}

func actList(c *cli.Context) error {
	var (
		client = getEtcdClient()
		opts   = []clientv3.OpOption{
			clientv3.WithPrefix(),
			clientv3.WithKeysOnly(),
			clientv3.WithSort(clientv3.SortByKey, clientv3.SortAscend),
		}
		printer = func(kv *mvccpb.KeyValue) {
			fmt.Printf("%s\n", kv.Key)
		}
		header string
	)

	if c.Bool("long") {
		header = " VER  CREATE-REV  MODIF-REV  KEY-NAME...\n-----+----------+----------+-------------"
		printer = func(kv *mvccpb.KeyValue) {
			fmt.Printf("%5d %10d %10d %s\n", kv.Version, kv.CreateRevision, kv.ModRevision, kv.Key)
		}
	}

	// Set up default params
	args := c.Args()
	if len(args) <= 0 {
		args = []string{""}
	}
	for i, a := range args {
		res, err := client.Get(ctx, a, opts...)
		checkErr(err)
		if len(args) > 1 || res.Count > 1 {
			if a != "" {
				logrus.Infof("Found %d keys in %s:", res.Count, a)
			} else {
				logrus.Infof("Found %d keys:", res.Count)
			}
		}
		if i == 0 && header != "" {
			fmt.Println(header)
		}
		for _, v := range res.Kvs {
			printer(v)
		}
	}
	return nil
}

func actTar(c *cli.Context) error {
	var (
		client  = getEtcdClient()
		optFile = c.String("f")
		optGzip = c.Bool("z")
		out     = io.WriteCloser(os.Stdout)
		err     error
	)

	// figure out output
	if optFile != "" {
		if out, err = os.Create(optFile); err != nil {
			return err
		}
		defer out.Close()
	} else {
		optFile = "STDOUT"
	}
	if optGzip {
		out = gzip.NewWriter(out)
		defer out.Close()
	}

	tw := tar.NewWriter(out)
	defer tw.Close()

	// Set up default params
	args := c.Args()
	if len(args) <= 0 {
		args = []string{""}
	}

	for _, a := range args {
		opts := []clientv3.OpOption{
			clientv3.WithPrefix(),
			clientv3.WithSort(clientv3.SortByKey, clientv3.SortAscend),
		}
		logrus.Debugf("Doing TAR(%s,%#v)...", a, opts)
		res, err := client.Get(ctx, a, opts...)
		checkErr(err)
		for _, v := range res.Kvs {
			header := new(tar.Header)
			header.Name = kvKey2FileName(v)
			header.Size = int64(len(v.Value))
			header.Mode = 0666
			header.ModTime = time.Now()
			if err := tw.WriteHeader(header); err != nil {
				return err
			}
			if _, err := io.Copy(tw, bytes.NewReader(v.Value)); err != nil {
				return err
			}
			logrus.Infof("Add %s [%d]...", v.Key, len(v.Value))
		}
	}

	logrus.Infof("Done writing %s", optFile)
	return nil
}

func actZip(c *cli.Context) error {
	var (
		client  = getEtcdClient()
		optFile = c.String("f")
		out     io.WriteCloser
		err     error
	)

	if optFile == "" {
		return fmt.Errorf("must specify output file (-f file)")
	} else if out, err = os.Create(optFile); err != nil {
		return err
	}

	// Set up default params
	args := c.Args()
	if len(args) <= 0 {
		args = []string{""}
	}

	zw := zip.NewWriter(out)
	defer func() {
		checkErr(zw.Close())
		out.Close()
	}()

	for _, a := range args {
		opts := []clientv3.OpOption{
			clientv3.WithPrefix(),
			clientv3.WithSort(clientv3.SortByKey, clientv3.SortAscend),
		}
		logrus.Debugf("Doing ZIP(%s,%#v)...", a, opts)
		res, err := client.Get(ctx, a, opts...)
		checkErr(err)
		var f io.Writer
		for _, v := range res.Kvs {
			f, err = zw.Create(kvKey2FileName(v))
			checkErr(err)
			_, err = f.Write(v.Value)
			checkErr(err)
			logrus.Infof("Add %s [%d]...", v.Key, len(v.Value))
		}
	}

	logrus.Infof("Done writing %s", optFile)
	return nil
}

func actDump(c *cli.Context) error {
	if c.NArg() <= 0 {
		return fmt.Errorf("must specify which keys to dump")
	}

	var (
		client    = getEtcdClient()
		optDir    = c.String("directory")
		optDecode = c.Bool("d64")
		optStrip  = c.Bool("strip")
		opts      = []clientv3.OpOption{
			clientv3.WithPrefix(),
			clientv3.WithSort(clientv3.SortByKey, clientv3.SortAscend),
		}
		logFmt = "Wrote %s [%d bytes]..."
	)

	if optDecode {
		logFmt = "Wrote %s [%d bytes, b64-decoded]..."
	}

	for _, a := range c.Args() {
		logrus.Debugf("Doing GET(%s,%#v)...", a, opts)
		res, err := client.Get(ctx, a, opts...)
		checkErr(err)
		for _, v := range res.Kvs {
			kk := kvKey2FileName(v)
			if optStrip {
				kk = path.Base(kk)
			}
			kk = path.Join(optDir, kk)
			if err := os.MkdirAll(path.Dir(kk), 0777); err != nil {
				return err
			}
			dbuf := v.Value
			if optDecode {
				dbuf = make([]byte, base64.StdEncoding.DecodedLen(len(v.Value)))
				if _, err := base64.StdEncoding.Decode(dbuf, v.Value); err != nil {
					return err
				}
			}
			if err := ioutil.WriteFile(kk, dbuf, 0666); err != nil {
				return err
			}
			logrus.Infof(logFmt, kk, len(dbuf))
		}
	}

	return nil
}

func actUpload(c *cli.Context) error {
	if c.NArg() <= 0 {
		return fmt.Errorf("must specify which directory to upload")
	}

	var (
		client    = getEtcdClient()
		optDir    = c.String("directory")
		optDirLen int
		optEncode = c.Bool("e64")
		optPrefix = c.String("prefix")
		logFmt    = "Put %s [%d]..."
		uploadFn  = func(fname string) error {
			dbuf, err := ioutil.ReadFile(fname)
			if err != nil {
				return err
			}
			logrus.Debugf("Read %s [%d] ...", fname, len(dbuf))
			if optEncode {
				ebuf := make([]byte, base64.StdEncoding.EncodedLen(len(dbuf)))
				base64.StdEncoding.Encode(ebuf, dbuf)
				dbuf = ebuf
			}
			kk := optPrefix + fname[optDirLen:]
			if _, err = client.Put(ctx, fileName2KvKey(kk), string(dbuf)); err == nil {
				logrus.Infof(logFmt, kk, len(dbuf))
			}
			return err
		}
		inFnameFn = func(a string) string { return a }
	)

	if optEncode {
		logFmt = "Put %s [%d, b64 encoded]..."
	}

	if optDir != "" {
		optDir = path.Clean(optDir)
		optDirLen = len(optDir) + 1
		inFnameFn = func(a string) string { return path.Join(optDir, a) }
	}

	for _, a := range c.Args() {
		a = inFnameFn(a)
		logrus.Debugf("Doing PUT(%s,XX)...", a)
		st, err := os.Stat(a)
		if err != nil {
			return err
		}
		if st.IsDir() {
			err = filepath.Walk(a, func(path string, info os.FileInfo, err error) error {
				if info.Mode().IsRegular() {
					if err = uploadFn(path); err != nil {
						return err
					}
				} else if info.Mode().IsDir() {
					// .. ignore
				} else {
					logrus.Warnf("Skipping '%s' (not a file or a directory)", a)
				}
				return nil
			})
			if err != nil {
				return err
			}
		} else if st.Mode().IsRegular() {
			// upload
			if err = uploadFn(a); err != nil {
				return err
			}
		} else {
			logrus.Warnf("Skipping '%s' (not a file or a directory)", a)
		}
	}
	return nil
}

func actRemove(c *cli.Context) error {
	if c.NArg() <= 0 {
		return fmt.Errorf("must specify which keys to remove")
	}

	var (
		client = getEtcdClient()
		txt    string
	)

	for _, a := range c.Args() {
		var opts []clientv3.OpOption
		ask := false
		if c.Bool("recursive") {
			// dumping subtree
			opts = []clientv3.OpOption{
				clientv3.WithPrefix(),
			}
			ask = !c.Bool("force")
		}
		logrus.Debugf("Doing DEL(%s,%#v)...", a, opts)
		if ask {
			if cnt := countKeys(a); cnt > 0 {
				fmt.Fprintf(logrus.StandardLogger().Out,
					"WARNING: About to delete %d keys in %s!  Continue [Y/*]? ", cnt, a)
				fmt.Scanln(&txt)
				if len(txt) < 1 || unicode.ToUpper(rune(txt[0])) != 'Y' {
					logrus.Error("Aborted.")
					os.Exit(1)
				}
			}
		}
		res, err := client.Delete(ctx, a, opts...)
		checkErr(err)
		logrus.Infof("Deleted %d keys.", res.Deleted)
	}

	return nil
}

func actGet(c *cli.Context) error {
	if c.NArg() <= 0 {
		return fmt.Errorf("must specify which keys to get")
	}

	var (
		client    = getEtcdClient()
		optDecode = c.Bool("d64")
		logFmt    = "Got %s [%d bytes]..."
	)

	if optDecode {
		logFmt = "Got %s [%d bytes, base64-decoded]..."
	}

	for _, a := range c.Args() {
		var opts []clientv3.OpOption
		if c.Bool("recursive") {
			// dumping subtree
			opts = []clientv3.OpOption{
				clientv3.WithPrefix(),
				clientv3.WithSort(clientv3.SortByKey, clientv3.SortAscend),
			}
		}
		logrus.Debugf("Doing GET(%s,%#v)...", a, opts)
		res, err := client.Get(ctx, a, opts...)
		checkErr(err)
		for i, v := range res.Kvs {
			dbuf := v.Value
			if optDecode {
				dbuf = make([]byte, base64.StdEncoding.DecodedLen(len(v.Value)))
				if _, err := base64.StdEncoding.Decode(dbuf, v.Value); err != nil {
					return err
				}
			}
			if i > 0 {
				os.Stdout.Write([]byte("\n"))
			}
			logrus.Infof(logFmt, v.Key, len(dbuf))
			os.Stdout.Write(dbuf)
		}
	}
	return nil
}

func actPut(c *cli.Context) error {
	if c.NArg() < 2 {
		return fmt.Errorf("must specify <file|-> <key>")
	}

	var (
		client    = getEtcdClient()
		optEncode = c.Bool("e64")
		optFile   = c.Args().Get(0)
		optKvPath = c.Args().Get(1)
		in        = io.ReadCloser(os.Stdin)
	)

	// figure out input
	if optFile != "-" {
		f, err := os.Open(optFile)
		if err != nil {
			return err
		}
		in = f
		defer f.Close()
	}

	dbuf, err := ioutil.ReadAll(in)
	if err != nil {
		return err
	}

	dbgOpts := ""
	if optEncode {
		dbgOpts = ", base64 encoded"
		ebuf := make([]byte, base64.StdEncoding.EncodedLen(len(dbuf)))
		base64.StdEncoding.Encode(ebuf, dbuf)
		dbuf = ebuf
	}

	logrus.Debugf("Doing PUT(%s,%#v)...", optFile, optKvPath)
	_, err = client.Put(ctx, fileName2KvKey(optKvPath), string(dbuf))
	checkErr(err)
	logrus.Infof("Put %s [%d%s]...", optKvPath, len(dbuf), dbgOpts)

	return nil
}

func main() {
	if s := os.Getenv("ETCD_LISTEN_CLIENT_URLS"); s != "" {
		opt.endpoints = s
	}

	app := cli.NewApp()
	app.Version = version
	app.Usage = "A dump/restore tool for etcd3."
	app.UsageText = app.Name + " <list|get|put|remove|dump|upload|tar|zip> [command options] [arguments...]\n\n" +
		`ENVIRONMENT VARIABLES:
   ETCD_LISTEN_CLIENT_URLS      Changes default endpoint`
	app.UseShortOptionHandling = true
	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:        "endpoints, e",
			Value:       opt.endpoints,
			Usage:       "set etcd endpoints",
			Destination: &opt.endpoints,
		},
		&cli.IntFlag{
			Name:        "timeout, T",
			Value:       opt.timeout,
			Usage:       "set timeout",
			Destination: &opt.timeout,
		},
		&cli.BoolFlag{
			Name:  "debug",
			Usage: "Turn on debug output",
		},
		&cli.BoolFlag{
			Name:  "quiet",
			Usage: "Suppress info messages",
		},
	}
	app.Before = func(c *cli.Context) error {
		if c.Bool("debug") {
			logrus.SetLevel(logrus.DebugLevel)
			logrus.Debug("Logging level set to DEBUG")
		} else if c.Bool("quiet") {
			logrus.SetLevel(logrus.WarnLevel)
		}
		return nil
	}

	app.Commands = []cli.Command{
		{
			Name:    "list",
			Aliases: []string{"ls"},
			Usage:   "list keys",
			Action:  actList,
			Flags: []cli.Flag{
				&cli.BoolFlag{
					Name:  "long, l",
					Usage: "use long output",
				},
			},
		},
		{
			Name:   "get",
			Usage:  "get keys",
			Action: actGet,
			Flags: []cli.Flag{
				&cli.BoolFlag{
					Name:  "d64",
					Usage: "perform base64 decoding",
				},
				&cli.BoolFlag{
					Name:  "recursive, r",
					Usage: "get keys recursively",
				},
			},
			UsageText: app.Name + " get key1 [key2...]",
		},
		{
			Name:   "put",
			Usage:  "put entry",
			Action: actPut,
			Flags: []cli.Flag{
				&cli.BoolFlag{
					Name:  "e64",
					Usage: "perform base64 encoding",
				},
			},
			UsageText: app.Name + " put <file|-> key",
		},
		{
			Name:    "remove",
			Aliases: []string{"rm"},
			Usage:   "remove keys",
			Action:  actRemove,
			Flags: []cli.Flag{
				&cli.BoolFlag{
					Name:  "force, f",
					Usage: "remove without prompting",
				},
				&cli.BoolFlag{
					Name:  "recursive, r",
					Usage: "delete recursively",
				},
			},
			UsageText:   app.Name + " rm key1 [key2/ ...]",
			Description: `Remove command removes keys (or directories of keys) from the EtcD.`,
		},
		{
			Name:   "dump",
			Usage:  "dump keys",
			Action: actDump,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "directory, C",
					Usage: "dump keys into given directory",
				},
				&cli.BoolFlag{
					Name:  "d64",
					Usage: "perform base64 decoding",
				},
				&cli.BoolFlag{
					Name:  "strip",
					Usage: "strip path(s) of the key",
				},
			},
			UsageText: app.Name + " dump [-C <dir>] key1 [key2...]",
		},
		{
			Name:    "upload",
			Aliases: []string{"up"},
			Usage:   "upload keys",
			Action:  actUpload,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "directory, C",
					Usage: "load keys from the given directory",
				},
				&cli.BoolFlag{
					Name:  "e64",
					Usage: "perform base64 encoding",
				},
				&cli.StringFlag{
					Name:  "prefix",
					Usage: "prefix the keys on upload",
				},
			},
			UsageText: app.Name + " upload [-C dir] dir1 [dir2...]",
		},
		{
			Name:   "tar",
			Usage:  "create TAR archive from EtcD keys",
			Action: actTar,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "f",
					Usage: "specify TAR filename",
				},
				&cli.BoolFlag{
					Name:  "z",
					Usage: "compress archive (GZip)",
				},
			},
			UsageText: app.Name + " tar [-f <file.tar>] [-z] key1 [key2...]",
		},
		{
			Name:   "zip",
			Usage:  "create ZIP archive from EtcD keys",
			Action: actZip,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "f",
					Usage: "specify ZIP filename",
				},
			},
			UsageText: app.Name + " zip -f <file.tar> key1 [key2...]",
		},
	}

	if err := app.Run(os.Args); err != nil {
		logrus.Error(err)
	}
}
