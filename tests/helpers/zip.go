package helpers

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2" //lint:ignore ST1001 this is a test file
	. "github.com/onsi/gomega"    //lint:ignore ST1001 this is a test file
)

func ZipDirectory(srcDir string) string {
	GinkgoHelper()

	file, err := os.CreateTemp("", "*.zip")
	if err != nil {
		Expect(err).NotTo(HaveOccurred())
	}
	defer file.Close()

	w := zip.NewWriter(file)
	defer w.Close()

	walker := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		fp, err := os.Open(path)
		if err != nil {
			return err
		}
		defer fp.Close()

		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		fh := &zip.FileHeader{
			Name: rel,
		}
		fh.SetMode(info.Mode())

		f, err := w.CreateHeader(fh)
		if err != nil {
			return err
		}

		_, err = io.Copy(f, fp)
		if err != nil {
			return err
		}

		return nil
	}
	err = filepath.Walk(srcDir, walker)
	Expect(err).NotTo(HaveOccurred())

	return file.Name()
}
