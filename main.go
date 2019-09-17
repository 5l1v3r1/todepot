package main

import (
	"bytes"
	"flag"
	"fmt"
	"github.com/cheggaaa/pb/v3"
	"github.com/google/uuid"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"
)

type FilePath struct {
	name string
	path string
	size int64
}

type FilesCollection struct {
	fileList []FilePath
	total    int64
}

func getFiles(info os.FileInfo, filesCollection *FilesCollection, urlBasePath string, fileBasePath string, searchHidden bool) {
	filePath := path.Join(fileBasePath, info.Name())
	fileName := path.Join(urlBasePath, info.Name())

	if info.Name()[0] != '.' || searchHidden {
		if info.IsDir() {
			files, err := ioutil.ReadDir(filePath)
			if err != nil {
				log.Printf("Error reading %s: %s", filePath, err)
			}

			for _, file := range files {
				getFiles(file, filesCollection, fileName, filePath, searchHidden)
			}
		} else {
			newFile := FilePath{name: fileName, path: filePath, size: info.Size()}
			filesCollection.fileList = append(filesCollection.fileList, newFile)
			filesCollection.total += info.Size()
		}
	}
}

func uploadFile(fileInfo FilePath, baseUrl string, bar *pb.ProgressBar) (bool, string) {
	client := &http.Client{}

	file, err := os.Open(fileInfo.path)
	if err != nil {
		return false, fmt.Sprintf("Error reading %s: %s", fileInfo.path, err)
	}

	uploadUrl := baseUrl + fileInfo.name

	var uploadBody io.Reader
	if fileInfo.size == 0 {
		uploadBody = nil
	} else {
		uploadBody = bar.NewProxyReader(file)
	}
	request, err := http.NewRequest("PUT", uploadUrl, uploadBody)

	if err != nil {
		return false, fmt.Sprintf("Failed to create request: %s", err)
	}

	request.ContentLength = fileInfo.size
	response, err := client.Do(request)

	defer func() {
		_ = response.Body.Close()
	}()

	if err != nil {
		return false, fmt.Sprintf("Failed to upload %s: %s", fileInfo.name, err)
	} else if response.StatusCode != 200 {
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(response.Body)

		return false, fmt.Sprintf("(%d) Failed to upload %s: %s", response.StatusCode, fileInfo.name, buf.String())
	}

	return true, ""
}

func uploadFiles(files FilesCollection, baseUrl string, printFiles bool, quiet bool, threads int) {
	tmpl := `[Files: {{string . "filecount"}} / {{string . "filetotal"}}] [Data: {{counters . }}] {{bar . }} {{percent . }} {{speed . }} {{rtime . "ETA %s"}} {{string . "filename"}}`
	bar := pb.New64(files.total)
	bar.SetTemplateString(tmpl)

	bar.Set("filecount", 0)
	bar.Set("filetotal", len(files.fileList))
	if !quiet {
		bar.Start()
	}

	var wg sync.WaitGroup
	threadSemaphore := make(chan struct{}, threads)

	var fileCount int64 = 0
	fileCountLock := &sync.Mutex{}

	for _, fileInfo := range files.fileList {
		wg.Add(1)

		go func(file FilePath, url string) {
			defer wg.Done()

			// Wait for new availability
			threadSemaphore <- struct{}{} // Lock
			defer func() {
				<-threadSemaphore // Unlock
			}()

			// Upload files and account for failures
			for i := 0; i < 3; i++ {
				succeeded, err := uploadFile(file, url, bar)
				if succeeded {
					break
				} else {
					log.Println(err)
					time.Sleep(10 * time.Second)
				}
			}

			// Update filecount
			fileCountLock.Lock()
			fileCount += 1
			bar.Set("filecount", fileCount)
			fileCountLock.Unlock()

			// Print filename
			if printFiles && !quiet {
				bar.Set("filename", file.path)
			}

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

		files := FilesCollection{fileList: make([]FilePath, 0), total: 0}
		getFiles(pathInfo, &files, "", path.Dir(basePath), *searchHidden)
		uploadFiles(files, url, *printFiles, *quiet, *uploadThreads)
	}
}
