package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

//做个http client从固定的网站下载文件，支持断点续传，下载的文件保存在当前目录
//cross over: 339.32M
var durl = "http://downza.91speed.vip/2022/04/21/crossover.zip"

//4.4G
//var durl = "http://officecdn.microsoft.com/pr/492350f6-3a01-4f97-b9c0-c7c6ddf67d60/media/zh-cn/ProPlus2019Retail.img"

func printRespInfo(resp *http.Response) {
	fmt.Println("Response Status:=", resp.Status)
	headers := resp.Header
	for header := range headers {
		v := headers[header]
		println(header + "=" + strings.Join(v, "|"))
	}
}

//实现goroutine处理大文件下载
func DownloadFileGo() {
	srcUrl, err := url.ParseRequestURI(durl)
	if err != nil {
		panic("ParseRequestURI failure!")
	}
	filename := path.Base(srcUrl.Path)

	//发送HTTP HEAD，看server是否支持Range
	myClient := http.Client{}
	req, err := http.NewRequest(http.MethodHead, durl, nil)
	if err != nil {
		panic("http.NewRequest HEAD failure..")
	}
	req.Header.Add("Range", "bytes=0-0")
	resp, err := myClient.Do(req)
	if err != nil {
		panic("myClient.Do: HTTP HEAD send failure.." + durl)
	}
	defer resp.Body.Close()

	//print resp headers
	//printRespInfo(resp)

	//resp header
	v := resp.Header.Get("Accept-Ranges")
	if v != "bytes" {
		downloadFileNoRange(filename, durl)
		return
	}

	//取得文件大小
	var total int64
	contentRange := resp.Header.Get("Content-Range")
	totalRange := strings.Split(contentRange, "/")
	if len(totalRange) >= 2 {
		total, _ = strconv.ParseInt(totalRange[1], 10, 64)
	}

	//用channel实现, 并发处理下载文件
	downloadFileGoroutine(filename, total, durl)

}

//不支持断点下传的服务器
func downloadFileNoRange(filename string, url string) {
	ntStart := time.Now()
	file1, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, os.ModePerm)
	if err != nil {
		panic("Open file failure..")
	}
	defer file1.Close()

	resp, err := http.Get(url)
	if err != nil {
		panic("http.Get failure..")
	}
	defer resp.Body.Close()
	v := resp.Header.Get("Content-Length")
	contentLength, _ := strconv.ParseInt(v, 10, 64)

	//分片存储到文件
	n := 0
	buf := make([]byte, 1024*1024)
	flag := 0
	for {
		num, err := resp.Body.Read(buf)
		if err != nil {
			if err == io.EOF {
				//break
				fmt.Println("resp.Body.Read EOF...")
			} else {
				fmt.Println("resp.Body.Read failure")
			}
			flag = 1
		}

		file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, os.ModePerm)
		file.Seek(int64(n), io.SeekStart)
		num, err = file.Write(buf[:num])
		n += num

		if flag == 1 {
			break
		}
	}
	//分片存储到文件

	if n != int(contentLength) {
		fmt.Println("The file size maybe wrong...")
	}

	ntEnd := time.Now()
	fmt.Printf("共用时：%v\n", ntEnd.Sub(ntStart))
}

//////////////////////////////////////////
/////////////////////////////////////////
var wg = sync.WaitGroup{}

//用channel实现多协程下载
type Range struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

var chanRange chan Range
var eachRangeLen int64 = 1024 * 1024 * 10 //10M

func downloadFileGoroutine(filename string, size int64, url string) {
	ntStart := time.Now()

	// 1.初始化管道
	if size > 1000000000 {
		eachRangeLen = 1024 * 1024 * 20 //20M
	}
	count := getChannCnt(size)
	chanRange = make(chan Range, count)

	//启动多个协程下载文件
	var rwLock sync.RWMutex

	//判断是否已经存在描述文件，是否是最初的下载
	file, err := os.Open(filename + "_tmp.txt")
	if err != nil { //没有描述文件，最开始的下载
		for i := 0; i < int(count/4); i++ {
			wg.Add(1)
			go DownloadFileRange(&rwLock, url, filename, size)

		}
		fmt.Printf("共启动%d个协程...\n", int(count/4))

		//把range切片,放进channel
		SliceSizeToRange(0, size)
		close(chanRange)

		wg.Wait()
	}
	file.Close()

	//断点续传的情况
	for {
		file, err = os.Open(filename)
		if err != nil {
			HandleError(err, "open file failure..")
			break
		}
		info, _ := file.Stat()
		if info.Size() >= size { //文件大小已经下载完毕
			break
		}
		tmpSize := size - info.Size()
		file.Close()

		count := getChannCnt(tmpSize)

		chanRange = make(chan Range, count)
		for i := 0; i < int(count/4); i++ {
			wg.Add(1)
			go DownloadFileRange(&rwLock, url, filename, size)
		}
		SliceTheUndlRanges(filename, size)
		close(chanRange)

		wg.Wait()
	}

	ntEnd := time.Now()
	fmt.Printf("共用时：%v\n", ntEnd.Sub(ntStart))

	//打印一下下载的文件大小是否一致
	file, _ = os.Open(filename)
	info, _ := file.Stat()
	fmt.Printf("下载的文件大小：%d/%d\n", info.Size(), size)
	file.Close()
}

func DownloadFileRange(rwLock *sync.RWMutex, url string, filename string, filesize int64) {

	for rangeGet := range chanRange {
		myClient := &http.Client{}
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			HandleError(err, "http.NewRequest")
			continue
		}
		if rangeGet.End == 0 {
			req.Header.Add("Range", fmt.Sprintf("bytes=%d-", rangeGet.Start))
		} else {
			req.Header.Add("Range", fmt.Sprintf("bytes=%d-%d", rangeGet.Start, rangeGet.End))
		}

		resp, err := myClient.Do(req)
		if err != nil {
			resp, err = myClient.Do(req)
		}
		if err != nil {
			HandleError(err, "myClient.Do")
			continue
		}

		//分片存储到文件
		seekStart := rangeGet.Start
		n := 0
		buf := make([]byte, 1024*1024)
		flag := 0
		for {
			num, err := resp.Body.Read(buf)
			if err != nil {
				if err == io.EOF {
					//fmt.Println("resp.Body.Read EOF...")
				} else {
					fmt.Println("resp.Body.Read failure")
				}
				flag = 1
			}

			rwLock.Lock()
			file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, os.ModePerm)
			if err != nil {
				HandleError(err, "os.OpenFile filename")
				continue
			}
			file.Seek(seekStart, io.SeekStart)
			num, err = file.Write(buf[:num])
			file.Close()
			rwLock.Unlock()
			n += num
			seekStart += int64(num)
			HandleError(err, "file write")

			if flag == 1 {
				break
			}
		}
		resp.Body.Close()
		//分片存储到文件

		//打印下载信息
		file, _ := os.Open(filename)
		info, _ := file.Stat()
		fmt.Printf("goroutine %d :\n,", GetGID())
		fmt.Printf("This download Range:%d-%d, The download filesize %d/%d\n", rangeGet.Start, rangeGet.Start+int64(n)-1, info.Size(), filesize)

		file.Close()

		//写入描述文件
		tmpfilename := filename + "_tmp.txt"
		var RangeStr string
		data, err := json.Marshal(&rangeGet)
		if err != nil {
			HandleError(err, "json marshal failure")
			continue
		}
		RangeStr = string(data) + "\n"

		rwLock.Lock()
		tmpfile, err := os.OpenFile(tmpfilename, os.O_CREATE|os.O_WRONLY, os.ModePerm)
		if err != nil {
			HandleError(err, "open file tmpfilename")
			continue
		}
		info, err = tmpfile.Stat()
		if err != nil {
			HandleError(err, "tmpfile.Stat")
			tmpfile.Close()
			continue
		}
		tmpfile.Seek(info.Size(), io.SeekStart)
		tmpfile.WriteString(RangeStr)
		rwLock.Unlock()
		tmpfile.Close()

	}
	wg.Done()
}

func HandleError(err error, why string) {
	if err != nil {
		fmt.Println(why, err)
	}
}

func SliceSizeToRange(rangeStart int64, rangeEnd int64) {
	var start int64 = rangeStart
	var end int64 = 0
	var n int = 0
	for {
		if start > rangeEnd {
			fmt.Printf("共切了%d片\n", n)
			break
		}

		end = start + eachRangeLen - 1
		if end > rangeEnd {
			end = rangeEnd
		}
		chanRange <- Range{start, end}

		start = end + 1
		n++
	}

}

func GetGID() uint64 {
	b := make([]byte, 64)
	b = b[:runtime.Stack(b, false)]
	b = bytes.TrimPrefix(b, []byte("goroutine "))
	b = b[:bytes.IndexByte(b, ' ')]
	n, _ := strconv.ParseUint(string(b), 10, 64)
	return n
}

func SliceTheUndlRanges(filename string, filesize int64) {

	//sort the ranges in file
	filename = filename + "_tmp.txt"
	file, err := os.Open(filename)
	if err != nil {
		return
	}
	defer file.Close()

	var line string
	var rangeArr = make([]Range, 1024)
	var i int = 0
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line = scanner.Text()
		err = json.Unmarshal([]byte(line), &rangeArr[i])
		if err != nil {
			HandleError(err, "json unmarshal failure..")
		}
		i++
	}

	var rangeSlice = rangeArr[:i]

	//sort the range
	for i := 0; i < len(rangeSlice); i++ {
		minIndex := i
		for j := i + 1; j < len(rangeSlice); j++ {
			if rangeSlice[j].Start < rangeSlice[minIndex].Start {
				minIndex = j
			}
		}
		if minIndex != i {
			rangeSlice[minIndex], rangeSlice[i] = rangeSlice[i], rangeSlice[minIndex]
		}
	}

	for i = 1; i < len(rangeSlice); i++ {
		if rangeSlice[i].Start > rangeSlice[i-1].End+1 {
			SliceSizeToRange(rangeSlice[i-1].End+1, rangeSlice[i].Start-1)
		}
	}
	v := rangeSlice[i-1].End
	if v != 0 && v < filesize {
		SliceSizeToRange(v+1, filesize)
	}
	return
}

func getChannCnt(size int64) (count int64) {
	count = size / eachRangeLen
	if count > 1000 {
		count = 1000
	}
	if count < 50 {
		count = 50
	}
	return count

}
