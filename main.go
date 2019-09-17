package main

import (
	"bytes"
	"flag"
	"fmt"
	"github.com/cheggaaa/pb"
	"github.com/google/uuid"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
)

type FilePath struct {
	name string
	path string
	size int64
}

func getFiles(info os.FileInfo, fileList []FilePath, urlBasePath string, fileBasePath string, searchHidden bool) []FilePath {
	filePath := path.Join(fileBasePath, info.Name())
	fileName := path.Join(urlBasePath, info.Name())

	if info.Name()[0] != '.' || searchHidden {
		if info.IsDir() {
			files, err := ioutil.ReadDir(filePath)
			if err != nil {
				log.Printf("Error reading %s: %s", filePath, err)
			}

			for _, file := range files {
				fileList = getFiles(file, fileList, fileName, filePath, searchHidden)
			}
		} else {
			fileList = append(fileList, FilePath{name: fileName, path: filePath, size: info.Size()})
		}
	}
	return fileList
}

func uploadFile(fileInfo FilePath, baseUrl string) {
	client := &http.Client{}

	file, err := os.Open(fileInfo.path)
	if err != nil {
		log.Printf("Error reading %s: %s", fileInfo.path, err)
	}

	uploadUrl := baseUrl + fileInfo.name

	var uploadBody io.Reader = file
	if fileInfo.size == 0 {
		uploadBody = nil
	}
	request, err := http.NewRequest("PUT", uploadUrl, uploadBody)

	if err != nil {
		log.Fatalf("Failed to create request: %s", err)
	}

	request.ContentLength = fileInfo.size
	response, err := client.Do(request)

	if err != nil {
		log.Fatalf("Failed to upload %s: %s", fileInfo.name, err)
	} else if response.StatusCode != 200 {
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(response.Body)

		log.Fatalf("(%d) Failed to upload %s: %s", response.StatusCode, fileInfo.name, buf.String())
	}
	err = response.Body.Close()
	if err != nil {
		log.Fatalf("Failed to close body of server response")
	}
}

func uploadFiles(files []FilePath, baseUrl string, printFiles bool, quiet bool, threads int) {
	bar := pb.New(len(files))
	if quiet {
		bar.SetWidth(0)
		bar.ShowBar = false
	} else {
		bar.Start()
	}

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, threads)

	for _, fileInfo := range files {
		wg.Add(1)

		go func(file FilePath, url string) {
			defer wg.Done()

			semaphore <- struct{}{} // Lock
			defer func() {
				<-semaphore // Unlock
			}()

			if printFiles && !quiet {
				bar.Postfix(" " + file.path)
			}
			bar.Increment()

			uploadFile(file, url)
		}(fileInfo, baseUrl)

	}
	wg.Wait()
	if !quiet {
		bar.Finish()
	}
}

func myUsage() {
	fmt.Printf("Usage: %s [URL] [PATH]...\n", os.Args[0])
	flag.PrintDefaults()
}

func main() {
	flag.Usage = myUsage
	uploadThreads := flag.Int("k", 8, "Number of simultaneous uploads")
	searchHidden := flag.Bool("a", false, "Include hidden files")
	printFiles := flag.Bool("v", false, "Print uploaded files")
	quiet := flag.Bool("q", false, "No output")

	flag.Parse()
	args := flag.Args()

	if flag.NArg() < 2 {
		flag.Usage()
		os.Exit(1)
	}

	url := strings.Replace(args[0], "[uuid]", uuid.New().String(), -1)
	if strings.Compare(url, args[0]) != 0 {
		fmt.Println(url)
	}

	for _, basePath := range args[1:] {
		basePath = path.Clean(basePath)

		if strings.Contains(basePath, "..") {
			log.Fatalf("Invalid path")
		}

		pathInfo, err := os.Stat(basePath)
		if err != nil {
			log.Fatalf("Couldn't read %s", basePath)
		}

		files := make([]FilePath, 0)
		files = getFiles(pathInfo, files, "", path.Dir(basePath), *searchHidden)
		uploadFiles(files, url, *printFiles, *quiet, *uploadThreads)
	}
}
