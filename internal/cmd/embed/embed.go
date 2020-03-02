package main

import (
	"archive/zip"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"io"
	"os"
	"path/filepath"
)

func zipPath(z *zip.Writer, root string) (err error) {
	stack := []string{root}
	for len(stack) > 0 {
		path := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		err = func() error {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()

			fi, err := file.Stat()
			if err != nil {
				return err
			}

			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}

			if rel != "." {
				fmt.Printf("%v -> %v\n", path, rel)
			}

			if fi.IsDir() {
				if rel != "." {
					_, err = z.Create(rel + "/")
					if err != nil {
						return err
					}
				}

				children, err := file.Readdir(-1)
				if err != nil {
					return err
				}

				for _, c := range children {
					stack = append(stack, filepath.Join(path, c.Name()))
				}

				return nil
			}

			w, err := z.Create(filepath.ToSlash(rel))
			if err != nil {
				return err
			}

			_, err = io.Copy(w, file)
			if err != nil {
				return err
			}

			return nil
		}()
		if err != nil {
			return err
		}
	}

	return nil
}

func createOutput(output string, r io.Reader, pkg, name string) error {
	var buf bytes.Buffer

	fmt.Fprintf(&buf, `package %v

import (
	"archive/zip"
	"bytes"
)

var %v = func(z *zip.Reader, err error) *zip.Reader {
	if err != nil {
		panic(err)
	}
	return z
}(zip.NewReader(bytes.NewReader([]byte{
`, pkg, name)

	var line [32]byte
	var size int
	for {
		n, err := r.Read(line[:])
		if (err != nil) && !errors.Is(err, io.EOF) {
			return err
		}
		size += n

		sep := "\t"
		for _, c := range line[:n] {
			fmt.Fprintf(&buf, "%v0x%X,", sep, c)
			sep = " "
		}
		fmt.Fprintln(&buf)

		if errors.Is(err, io.EOF) {
			break
		}
	}
	fmt.Fprintf(&buf, "}), %v))", size)

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return err
	}

	file, err := os.Create(output)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, bytes.NewReader(formatted))
	return err
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %v [options] <path>\n", os.Args[0])
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Options:")
		flag.PrintDefaults()
	}
	output := flag.String("o", "embed.go", "Output file.")
	pkg := flag.String("pkg", "main", "Package name of produced file.")
	name := flag.String("var", "embed", "Variable name of zip file.")
	flag.Parse()

	path := flag.Arg(0)
	if path == "" {
		flag.Usage()
		os.Exit(2)
	}

	var buf bytes.Buffer
	z := zip.NewWriter(&buf)

	err := zipPath(z, path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	err = z.Close()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	err = createOutput(*output, &buf, *pkg, *name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
