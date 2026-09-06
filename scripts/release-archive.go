//go:build ignore

package main

import (
	"archive/tar"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func main() {
	root := flag.String("root", "", "directory to archive")
	output := flag.String("output", "", "archive path")
	prefix := flag.String("prefix", "", "archive path prefix")
	epoch := flag.Int64("epoch", 0, "fixed Unix timestamp")
	flag.Parse()

	if *root == "" || *output == "" || *epoch <= 0 {
		fatalf("--root, --output, and a positive --epoch are required")
	}

	rootPath, err := filepath.Abs(*root)
	if err != nil {
		fatalf("resolve root: %v", err)
	}
	var paths []string
	err = filepath.Walk(rootPath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path != rootPath {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		fatalf("walk archive root: %v", err)
	}
	sort.Strings(paths)

	out, err := os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		fatalf("create archive: %v", err)
	}
	defer out.Close()

	zw, err := gzip.NewWriterLevel(out, gzip.BestCompression)
	if err != nil {
		fatalf("create gzip writer: %v", err)
	}
	fixedTime := time.Unix(*epoch, 0).UTC()
	zw.Header.ModTime = fixedTime
	zw.Header.OS = 255
	tw := tar.NewWriter(zw)

	for _, path := range paths {
		info, statErr := os.Lstat(path)
		if statErr != nil {
			fatalf("stat %s: %v", path, statErr)
		}
		rel, relErr := filepath.Rel(rootPath, path)
		if relErr != nil {
			fatalf("relative path: %v", relErr)
		}
		name := filepath.ToSlash(filepath.Join(*prefix, rel))
		if info.IsDir() {
			name += "/"
		}
		link := ""
		if info.Mode()&os.ModeSymlink != 0 {
			link, err = os.Readlink(path)
			if err != nil {
				fatalf("read symlink %s: %v", path, err)
			}
		}
		header, headerErr := tar.FileInfoHeader(info, link)
		if headerErr != nil {
			fatalf("header %s: %v", path, headerErr)
		}
		header.Name = strings.TrimPrefix(name, "./")
		header.ModTime = fixedTime
		header.AccessTime = time.Time{}
		header.ChangeTime = time.Time{}
		header.Uid = 0
		header.Gid = 0
		header.Uname = "root"
		header.Gname = "root"
		if err = tw.WriteHeader(header); err != nil {
			fatalf("write header %s: %v", path, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		file, openErr := os.Open(path)
		if openErr != nil {
			fatalf("open %s: %v", path, openErr)
		}
		_, copyErr := io.Copy(tw, file)
		closeErr := file.Close()
		if copyErr != nil {
			fatalf("archive %s: %v", path, copyErr)
		}
		if closeErr != nil {
			fatalf("close %s: %v", path, closeErr)
		}
	}

	if err = tw.Close(); err != nil {
		fatalf("close tar writer: %v", err)
	}
	if err = zw.Close(); err != nil {
		fatalf("close gzip writer: %v", err)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "release archive: "+format+"\n", args...)
	os.Exit(1)
}
