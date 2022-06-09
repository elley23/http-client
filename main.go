package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
)

func check(err error) {
	if err != nil {
		panic(err)
	}
}

//访问本机的目录系统
func staticTest() {
	myClient := http.Client{}
	resp, err := myClient.Get("http://127.0.0.1:81/static/")
	//resp, err := myClient.Get("http://www.baidu.com") //提交HTTP_GET请求
	check(err)
	defer resp.Body.Close()
	//header, err := ioutil.ReadAll(resp.Header)
	body, err := ioutil.ReadAll(resp.Body)
	//fmt.Println("%s", string(header))
	fmt.Println("%s", string(body[:]))
}

//访问自己做的http server，下载C:/wuliu/documents/test/testa.jpg到testb.jpg,支持断点续传
func breakpointDownload() {
	srcFileUrl := "http://localhost:81/pictureDownload/"
	fileName := "picture"
	destFile := "C:/wuliu/documents/test/" + fileName + ".jpg"
	tempFile := "C:/wuliu/documents/test/" + fileName + "_temp.txt"
	nomalRespCode := "206 Partial Content"
	errRespCode := "416 Requested Range Not Satisfiable"
	buffByte := int64(1024*200 - 1)

	//fmt.Printf("destFile:%v\n", destFile)
	//fmt.Printf("tempFile:%v\n", tempFile)
	//fmt.Printf("srcFileUrl:%v\n", srcFileUrl)

	file2, err := os.OpenFile(destFile, os.O_CREATE|os.O_WRONLY, os.ModePerm)
	check(err)
	defer file2.Close()

	//临时文件,读取临时文件的信息，根据seek
	file3, err := os.OpenFile(tempFile, os.O_CREATE|os.O_RDWR, os.ModePerm)
	check(err)

	file3.Seek(0, io.SeekStart)
	buf := make([]byte, 100, 100)
	n1, err := file3.Read(buf)
	if err != io.EOF {
		check(err)
	}
	countStr := string(buf[:n1]) //读取的byte转换成字符串
	//每次记录的byte数用’/'进行分隔,把读出来的countStr根据“/"分隔符变成数组
	countArr := strings.Split(countStr, "/")
	var count, total, end int64 = 0, 0, 0 //count:已经下载的，total：总文件大小
	var bfb = "0%"
	var countf, totalf float64 = 0.0001, 10000

	if len(countArr) >= 3 {
		count, _ = strconv.ParseInt(countArr[0], 10, 64)
		total, _ = strconv.ParseInt(countArr[1], 10, 64)
		bfb = countArr[2]
	}

	fmt.Println(fmt.Sprintf("开始下载，已下载：%d，总共：%d，进度：%s", count, total, bfb))

	for {
		req, err := http.NewRequest(http.MethodGet, srcFileUrl, nil)
		check(err)

		if total != 0 && total < count+buffByte {
			end = total
		} else {
			end = count + buffByte
		}
		range01 := fmt.Sprintf("bytes=%d-%d", count, end)
		req.Header.Set("Range", range01)

		myClient := &http.Client{} //{Timeout: 900 * time.Second}
		//myClient.Timeout = time.Second * 600
		resp, err := myClient.Do(req)
		check(err)
		defer resp.Body.Close()

		//打印Response的Content-Range：
		//v := resp.Header.Get("Content-Range")
		//fmt.Printf("Content-Range in Response: %v\n", v)

		contentRange := resp.Header.Get("Content-Range")
		totalRange := strings.Split(contentRange, "/")
		if len(totalRange) >= 2 {
			total, _ = strconv.ParseInt(totalRange[1], 10, 64)
		}

		//206 Partial Content, 416 Requested Range Not Satisfiable
		//fmt.Println("resp.Status:", resp.Status)

		if resp.Status != nomalRespCode { //不能是200吗？？？？？？
			if resp.Status == errRespCode {
				//fmt.Println("文件下载完毕..")
				//file3.Close()
			} else {
				//fmt.Printf("文件传输异常报错：err=%s\n", resp.Status)
			}
			fmt.Printf("文件传输异常报错：err=%s\n", resp.Status)
			file3.Close()
			break
		}

		n3 := -1                        //写入的数据量
		file2.Seek(count, io.SeekStart) //设置要存储的文件的下一次写的光标起点
		data, err := ioutil.ReadAll(resp.Body)
		//fmt.Println("data size:", len(data))

		n3, _ = file2.Write(data)
		count += int64(n3)
		countf = float64(count)
		totalf = float64(total)
		bfb = fmt.Sprintf("%.2f%", countf/totalf*100)
		fmt.Println("本次读取了：", n3, "总共已下载：", count, "文件总大小：", total, "下载进度：", bfb)

		w := strconv.Itoa(int(count)) + "/" + strconv.Itoa(int(total)) + "/" + bfb
		file3.Seek(0, io.SeekStart)
		file3.WriteString(w)

		if count >= total {
			fmt.Println("文件下载完毕..")
			file3.Close()
			break
		}

		//手工断电
		if count > 1024*1000 {
			panic("断电了")
		}
	}
}
func fileTest() {
	err := os.MkdirAll("./templates", 0777)
	if err != nil {
		fmt.Printf("%s", err)
	} else {
		fmt.Print("创建目录成功!")
	}
	filename := "templates/test.a"

	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, os.ModePerm)
	if err != nil {
		println(err)
	}
	println(file)
}

func main() {
	//staticTest()
	//breakpointDownload()
	//downloadFile()
	//dir, _ := os.Getwd()
	//fmt.Println(dir)
	/*	err := os.RemoveAll("./templates")
		if err != nil {
			fmt.Println("remove all failure...")
			fmt.Println(err)
		}*/
	downloadFileGo()
}
