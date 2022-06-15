package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
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

//downloadFile,做个http client从固定的网站下载文件，支持断点续传，下载的文件保存在当前目录
//cross over: 339.32M
//http://downza.91speed.vip/2022/04/21/crossover.zip
//win10 3.61G
//https://windows.xnayw.cn/download/win1064.html

//var durl = "http://downza.91speed.vip/2022/04/21/crossover.zip"

//4.4G
var durl = "http://officecdn.microsoft.com/pr/492350f6-3a01-4f97-b9c0-c7c6ddf67d60/media/zh-cn/ProPlus2019Retail.img"

func downloadFileWithRange() {
	srcUrl, err := url.ParseRequestURI(durl)
	check(err)

	filename := path.Base(srcUrl.Path)
	tmpFile := filename + "_bak"
	//fmt.Println("[*]Filename:" + filename)
	//fmt.Println("[*]tmpFile:" + tmpFile)
	var count, total int64 = 0, 0
	var countf, totalf float64 = 0.0001, 10000
	bfb := "0%"
	//var thisCount int64 = 0

	file1, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, os.ModePerm)
	check(err)
	defer file1.Close()
	fileBak, err := os.OpenFile(tmpFile, os.O_CREATE|os.O_RDWR, os.ModePerm)
	check(err)
	/*n1, err := fileBak.Stat()
	check(err)
	if n1.Size() > 0 {
		bp = 1
	}*/

	//读取临时记录下载情况的文件
	fileBak.Seek(0, io.SeekStart)
	buf := make([]byte, 100, 100)
	n1, err := fileBak.Read(buf)
	if err == io.EOF {
	}
	countStr := string(buf[:n1])
	countArr := strings.Split(countStr, "/")
	if len(countArr) >= 3 {
		count, _ = strconv.ParseInt(countArr[0], 10, 64)
		total, _ = strconv.ParseInt(countArr[1], 10, 64)
		bfb = countArr[2]
	}

	ntStart := time.Now()
	nt := time.Now().Format("2006-01-02 03:04:05")
	fmt.Println(fmt.Sprintf("[%s] 开始下载，已下载：%d，总共：%d，进度：%s", nt, count, total, bfb))

	//resp, err := http.Get(durl)
	//check(err)
	//defer resp.Body.Close()
	myClient := http.Client{} //{Timeout: time.Second * 60}
	req, err := http.NewRequest(http.MethodGet, durl, nil)
	check(err)
	if count == 0 {
		req.Header.Add("Range", "bytes=0-")
	} else {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", count, total))
	}
	resp, err := myClient.Do(req)
	check(err)
	defer resp.Body.Close()

	//用ioutil读取文件，如果断网，文件读取失败,不能panic,否则不能实现断点续传了
	//panic: read tcp 192.168.0.103:55182->120.253.255.33:443: wsarecv: An established connection was aborted
	//by the software in your host machine.

	//resp header
	contentRange := resp.Header.Get("Content-Range")
	totalRange := strings.Split(contentRange, "/")
	if len(totalRange) >= 2 {
		total, _ = strconv.ParseInt(totalRange[1], 10, 64)
	} else {
		value := resp.Header.Get("Content-Length")
		total, _ = strconv.ParseInt(value, 10, 64)
		//fmt.Printf("the content-length: %d\n", total)
	}

	//print resp headers
	/*headers := resp.Header
	for header := range headers {
		v := headers[header]
		println(header + "=" + strings.Join(v, "|"))
	}*/

	//resp body
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("ioutil read resp.body failed, please check your network..")
		//panic("Network failure!..")
	}
	file1.Seek(count, io.SeekStart)
	n2, err := file1.Write(data)
	check(err)

	count += int64(n2)
	countf = float64(count)
	totalf = float64(total)
	bfb = fmt.Sprintf("%.2f", countf/totalf*100)
	fmt.Println(fmt.Sprintf("本次读取了：%d, 总共已下载：%d, 文件总大小：%d, 下载进度：%s", n2, count, total, bfb))

	w := strconv.Itoa(int(count)) + "/" + strconv.Itoa(int(total)) + "/" + bfb
	fileBak.Seek(0, io.SeekStart)
	fileBak.WriteString(w)
	if count == total {
		ntEnd := time.Now()
		nt := time.Now().Format("2006-01-02 03:04:05")
		fmt.Printf("[%s] 文件下载完毕...耗时[%v]\n", nt, ntEnd.Sub(ntStart))
		fileBak.Close()
	}
	return

}
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

	//发生HTTP HEAD，看server是否支持Range
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

	//并发处理下载文件
	//downloadFileGoroutine(filename, total, durl)

	//用channel实现, 并发处理下载文件
	downloadFileGoroutine1(filename, total, durl)

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

	//一次性存取
	/*
		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			panic("ioutil.Readall(resp.Body) failure...")
		}
		n, err := file1.Write(data)
		if err != nil {
			panic("file.Write failure...")
		}
		if n != int(contentLength) {
			fmt.Println("The file size maybe wrong...")
		}

	*/
	//一次性存取

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

func downloadOneFile(url string, filename string, rangeStart, rangeEnd int) {
	var rangeValue string

	defer wg.Done()

	if rangeStart <= rangeEnd {
		rangeValue = fmt.Sprintf("bytes=%d-%d", rangeStart, rangeEnd)
	} else if rangeEnd == 0 {
		rangeValue = fmt.Sprintf("bytes=%d-", rangeStart)
	} else if rangeStart > rangeEnd {
		fmt.Println("Range error...")
		return
	}

	//创建、发生http.get
	myClient := http.Client{}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		fmt.Println("http new request failure..")
		return
	}
	req.Header.Add("Range", rangeValue)
	resp, err := myClient.Do(req)
	if err != nil {
		fmt.Println("http get failure..")
		return
	}
	defer resp.Body.Close()

	// 创建硬盘文件，名字为 源文件 + 第 number 个协程
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, os.ModePerm)
	if err != nil {
		fmt.Println("Create file failure...")
		return
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		fmt.Println("copy resp.body to file failure...")
		return
	}

}

var goroutine_number int = 10
var wg = sync.WaitGroup{}

func downloadFileGoroutine(filename string, size int64, url string) {
	ntStart := time.Now()
	eachFileLen := int(size) / goroutine_number

	//临时文件目录
	//myDir, _ := os.Getwd()
	//tmpDir := fmt.Sprintf("%s/templates/", myDir)
	//os.Mkdir(tmpDir, 0777)
	//defer os.Remove(tmpDir)
	err := os.MkdirAll("./templates", 0777)
	if err != nil {
		fmt.Printf("%s", err)
	} else {
		//fmt.Print("创建目录成功!")
	}
	defer os.RemoveAll("./templates")

	//file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, os.ModePerm)
	//if err != nil {
	//	println(err)
	//}
	//println(file)

	//启动并发
	wg.Add(goroutine_number)
	rangeStart := 0
	for i := 0; i < goroutine_number; i++ {
		tmpFilename := fmt.Sprintf("templates/%s_%d", filename, i)
		rangeEnd := rangeStart + eachFileLen
		if i == goroutine_number-1 {
			rangeEnd = 0
		}

		go downloadOneFile(url, tmpFilename, rangeStart, rangeEnd)
		rangeStart += eachFileLen + 1
	}

	wg.Wait()

	var offset int64 = 0
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, os.ModePerm)
	if err != nil {
		fmt.Println("mergeFiles: create destination file failure...")
		return
	}
	defer file.Close()
	for i := 0; i < goroutine_number; i++ {
		tmpFilename := fmt.Sprintf("templates/%s_%d", filename, i)
		n, err := mergeFiles(file, offset, tmpFilename)
		if err != nil {
			fmt.Println("merge files failure...")
			return
		}
		offset += n
	}

	ntEnd := time.Now()
	fmt.Printf("共用时：%v\n", ntEnd.Sub(ntStart))

	/*err = os.RemoveAll("./templates")
	if err != nil {
		fmt.Println("remove all failure...")
		fmt.Println(err)
	}*/
}

func mergeFiles(file *os.File, offset int64, srcFilename string) (n int64, err error) {
	srcFile, err := os.Open(srcFilename)
	if err != nil {
		fmt.Println("mergeFiles: open templates file failure...")
		return 0, err
	}
	defer srcFile.Close()

	file.Seek(offset, io.SeekStart)
	n1, err := io.Copy(file, srcFile)
	if err != nil {
		fmt.Println("mergeFiles: copy templates file failure...")
		return 0, err

	}
	return n1, nil
}

//////////////////////////////////////////
/////////////////////////////////////////
//用channel实现多协程下载
type Range struct {
	start int64
	end   int64
}

var chanRange chan Range
var done chan struct{}

var eachRangeLen int64 = 1024 * 1024 * 10 //10M

func downloadFileGoroutine1(filename string, size int64, url string) {
	ntStart := time.Now()

	// 1.初始化管道
	if size > 1000000000 {
		eachRangeLen = 1024 * 1024 * 20 //20M
	}
	count := size / eachRangeLen
	if count > 1000 {
		count = 1000
	}
	if count < 50 {
		count = 50
	}
	chanRange = make(chan Range, count)

	//SliceSizeToRange(size)

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

		//wg.Add(1)
		//go checkDownloadDone()

		//把range切片,放进channel
		SliceSizeToRange(0, size)
		close(chanRange)

		wg.Wait()
	}
	file.Close()

	//断点续传的情况
	cnt := (size / eachRangeLen) * 2
	if cnt < 10 {
		cnt = 10
	}
	//rangeArr := make([]Range, cnt)
	//var rangeCnt int = 0

	rangeArr, tmpSize := GetMissingRanges(filename, size)
	if tmpSize != 0 {
		chanRange = make(chan Range, count)
		for i := 0; i < int(count/4); i++ {
			wg.Add(1)
			go DownloadFileRange(&rwLock, url, filename, size)
		}
		//rangeSlice := rangeArr[:rangeCnt]
		SliceTheMissingRanges(size, rangeArr)
		close(chanRange)

		wg.Wait()
	}

	ntEnd := time.Now()
	fmt.Printf("共用时：%v\n", ntEnd.Sub(ntStart))

	//打印一下下载的文件大小是否一直
	file, _ = os.Open(filename)
	info, _ := file.Stat()
	fmt.Printf("下载的文件大小：%d/%d\n", info.Size(), size)
}

func checkDownloadDone() {
	for _ = range done {
		close(chanRange)
		break
	}
	close(done)
	wg.Done()
}

func DownloadFileRange(rwLock *sync.RWMutex, url string, filename string, filesize int64) {

	for rangeGet := range chanRange {
		myClient := &http.Client{}
		req, err := http.NewRequest(http.MethodGet, url, nil)
		HandleError(err, "http.NewRequest")
		if rangeGet.end == 0 {
			req.Header.Add("Range", fmt.Sprintf("bytes=%d-", rangeGet.start))
		} else {
			req.Header.Add("Range", fmt.Sprintf("bytes=%d-%d", rangeGet.start, rangeGet.end))
		}

		resp, err := myClient.Do(req)
		if err != nil {
			resp, err = myClient.Do(req)
		}
		if err != nil {
			//chanRange <- Range{rangeGet.start, rangeGet.end}
			HandleError(err, "myClient.Do")
			return
		}
		//defer resp.Body.Close()

		//print resp
		//printRespInfo(resp)

		//一次存储到文件
		/*
			bytes, err := ioutil.ReadAll(resp.Body)
			HandleError(err, "ioutil.ReadAll resp.body")

				rwLock.Lock()
				file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, os.ModePerm)
				HandleError(err, "os.OpenFile filename")
				file.Seek(rangeGet.start, io.SeekStart)
				n, err := file.Write(bytes)
				rwLock.Unlock()
				HandleError(err, "file write")
		*/
		//一次存储到文件

		//分片存储到文件
		seekStart := rangeGet.start
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

			rwLock.Lock()
			file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, os.ModePerm)
			HandleError(err, "os.OpenFile filename")
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
		//分片存储到文件

		//打印下载信息
		file, _ := os.Open(filename)
		info, _ := file.Stat()
		fmt.Printf("goroutine %d :\n,", GetGID())
		fmt.Printf("This download Range:%d-%d, The download filesize %d/%d\n", rangeGet.start, rangeGet.start+int64(n)-1, info.Size(), filesize)

		file.Close()

		//fmt.Printf("%s writes size is %d\n", filename, n)

		//写入描述文件
		tmpfilename := filename + "_tmp.txt"
		var RangeStr string
		if int64(n) >= rangeGet.end-rangeGet.start {
			RangeStr = strconv.FormatInt(rangeGet.start, 10) + "-" + strconv.FormatInt(rangeGet.end, 10) + "\n"
		} else {
			RangeStr = strconv.FormatInt(rangeGet.start, 10) + "-" + strconv.FormatInt(rangeGet.start+int64(n)-1, 10) + "\n"
		}
		rwLock.Lock()
		tmpfile, err := os.OpenFile(tmpfilename, os.O_CREATE|os.O_WRONLY, os.ModePerm)
		HandleError(err, "open file tmpfilename")
		info, err = tmpfile.Stat()
		HandleError(err, "tmpfile.Stat")
		tmpfile.Seek(info.Size(), io.SeekStart)
		tmpfile.WriteString(RangeStr)
		rwLock.Unlock()
		tmpfile.Close()

		resp.Body.Close()
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

func SliceTheMissingRanges(filesize int64, rangeArr []Range) {

	i := 0
	for i = 1; i < len(rangeArr); i++ {
		if rangeArr[i].start > rangeArr[i-1].end+1 {
			SliceSizeToRange(rangeArr[i-1].end+1, rangeArr[i].start-1)
		}
	}
	v := rangeArr[i-1].end
	if v != 0 && v < filesize {
		SliceSizeToRange(v+1, filesize)
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

func GetMissingRanges(filename string, filesize int64) (rangeArr []Range, size int64) {

	//sort the ranges in file
	filename = filename + "_tmp.txt"
	file, err := os.Open(filename)
	if err != nil {
		return nil, 0
	}

	var buf [1024]byte
	var content []byte

	for {
		_, err := file.Read(buf[:])
		if err != nil {
			if err == io.EOF {
				break
			}
		}
		content = append(content, buf[:]...)

	}
	info, _ := file.Stat()

	content1 := content[:info.Size()-1]
	file.Close()

	str := string(content1)
	bytesArr := strings.Split(str, "\n")

	rangeSlice := make([]Range, len(bytesArr))
	//rangeSlice := rangeArr[:len(bytesArr)]
	for i := 0; i < len(bytesArr); i++ {
		rangeStr := strings.Split(bytesArr[i], "-")
		rangeSlice[i].start, _ = strconv.ParseInt(rangeStr[0], 10, 64)
		rangeSlice[i].end, _ = strconv.ParseInt(rangeStr[1], 10, 64)
	}
	for i := 0; i < len(rangeSlice); i++ {
		minIndex := i
		for j := i + 1; j < len(rangeSlice); j++ {
			if rangeSlice[j].start < rangeSlice[minIndex].start {
				minIndex = j
			}
		}
		if minIndex != i {
			rangeSlice[minIndex], rangeSlice[i] = rangeSlice[i], rangeSlice[minIndex]
		}
	}

	//find the missing ranges

	size = 0
	for i := 1; i < len(rangeSlice); i++ {
		if rangeSlice[i].start > rangeSlice[i-1].end+1 {
			//chanRange <- Range{rangeSlice[i-1].end + 1, rangeSlice[i].start - 1}
			//fmt.Println(rangeSlice[i-1].end+1, rangeSlice[i].start-1)
			size += rangeSlice[i].start - rangeSlice[i-1].end - 1
		}
	}

	v := rangeSlice[len(rangeSlice)-1].end
	if v != 0 && v < filesize {
		size += filesize - v
	}

	return rangeSlice, size
}
