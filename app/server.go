package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

const (
	CRLF   = "\r\n"
	HTTP11 = "HTTP/1.1"
)

type Status struct {
	method   string
	path     string
	protocol string
}

type Headers map[string]string

type HTTPRequest struct {
	status  Status
	headers Headers
	body    string
}

func writeStatusLine(conn net.Conn, statusCode int, statusText string) {
	fmt.Fprintf(conn, "%s %d %s%s", HTTP11, statusCode, statusText, CRLF)
}
func writeHeader(conn net.Conn, key, value string) {
	fmt.Fprintf(conn, "%s: %s%s", key, value, CRLF)
}
func endHeaders(conn net.Conn) {
	fmt.Fprint(conn, CRLF)
}
func writeContent(conn net.Conn, contentType string, content string, contentEncoding string) {
	writeHeader(conn, "Content-Type", contentType)
	if strings.Contains(contentEncoding, "gzip") {
		var buffer bytes.Buffer
		w := gzip.NewWriter(&buffer)
		w.Write([]byte(content))
		w.Close()
		content = buffer.String()
		writeHeader(conn, "Content-Length", fmt.Sprintf("%d", len(content)))
		writeHeader(conn, "Content-Encoding", "gzip")
		endHeaders(conn)
		fmt.Fprint(conn, content)
		return
	}
	writeHeader(conn, "Content-Length", fmt.Sprintf("%d", len(content)))
	endHeaders(conn)
	fmt.Fprint(conn, content)
}

func main() {
	fmt.Println("Logs from your program will appear here!")

	listener, err := net.Listen("tcp", "0.0.0.0:4221")
	if err != nil {
		fmt.Println("Failed to bind to port 4221", err.Error())
		os.Exit(1)
	}
	fmt.Printf("Listening on: %v\n", listener.Addr().String())

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Error accepting connection: ", err.Error())
			os.Exit(1)
		}
		fmt.Println("Accepted connection from: ", conn.RemoteAddr().String())

		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()

	request := createRequestObject(conn)

	switch request.status.method {
	case "GET":
		getFunc(conn, request)
	case "POST":
		postFunc(conn, request)
	default:
		writeStatusLine(conn, 405, "Method Not Allowed")
		endHeaders(conn)
	}
}

func getFunc(conn net.Conn, request HTTPRequest) {
	encoding := request.headers["accept-encoding"]
	switch request.status.path {
	case "/", "":
		writeStatusLine(conn, 200, "OK")
		endHeaders(conn)
	default:
		url := strings.Split(request.status.path, "/")
		firstParam := url[1]
		switch firstParam {
		case "echo":
			var message string
			if len(url) > 2 && url[2] != "" {
				message = url[2]
			} else {
				message = "No message provided"
			}
			writeStatusLine(conn, 200, "OK")
			writeContent(conn, "text/plain", message, encoding)
		case "user-agent":
			if val, ok := request.headers[firstParam]; ok {
				writeStatusLine(conn, 200, "OK")
				writeContent(conn, "text/plain", val, encoding)
			}
		case "files":
			if len(url) > 2 {
				fileName := url[2]
				content, err := extractFile(fileName)
				if err != nil {
					fmt.Println("Error while finding file: ", err.Error())
					writeStatusLine(conn, 404, "Not Found")
					endHeaders(conn)
					os.Exit(1)
				}
				writeStatusLine(conn, 200, "OK")
				writeContent(conn, "application/octet-stream", content, encoding)
			}
		default:
			writeStatusLine(conn, 404, "Not Found")
			endHeaders(conn)
		}
	}
}

func postFunc(conn net.Conn, request HTTPRequest) {
	switch request.status.path {
	case "/", "":
		writeStatusLine(conn, 404, "Not Found")
		endHeaders(conn)
	default:
		url := strings.Split(request.status.path, "/")
		firstParam := url[1]
		switch firstParam {
		case "files":
			if len(url) > 2 {
				fileName := url[2]
				err := writeFile(fileName, request.body)
				if err != nil {
					fmt.Println("Error while Creating file: ", err.Error())
					writeStatusLine(conn, 400, "Bad Request")
					endHeaders(conn)
					os.Exit(1)
				}
				writeStatusLine(conn, 201, "Created")
				endHeaders(conn)
			}
		default:
			writeStatusLine(conn, 404, "Not Found")
			endHeaders(conn)
		}
	}
}

func createRequestObject(conn net.Conn) HTTPRequest {
	reader := bufio.NewReader(conn)
	requestLine, err := reader.ReadString('\n')
	if err != nil {
		writeStatusLine(conn, 400, "Bad Request")
		endHeaders(conn)
		os.Exit(1)
	}
	statusPayload := createStatusObject(conn, requestLine)

	headerPayload := make(map[string]string)

	for {
		headerLines, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("Error parsing headers: ", err.Error())
			break
		}

		if headerLines == "\r\n" {
			fmt.Println("All the headers are parsed")
			break
		}

		splitHeader := strings.SplitN(headerLines, ":", 2)
		headerPayload[strings.ToLower(strings.TrimSpace(splitHeader[0]))] = strings.TrimSpace(splitHeader[1])
	}

	var bodyPayload string
	if val, ok := headerPayload["content-length"]; ok {
		contentLength, err := strconv.Atoi(val)
		if err != nil {
			fmt.Println("Error while parsing contentLength: ", err.Error())
		}
		if contentLength > 0 {
			data := make([]byte, contentLength)
			length, err := reader.Read(data)
			if err != nil {
				fmt.Println("Error while reading data: ", err.Error())
				os.Exit(1)
			}
			bodyPayload = string(data[:length])
		}
	}

	return HTTPRequest{statusPayload, headerPayload, bodyPayload}
}

func createStatusObject(conn net.Conn, requestLine string) Status {
	fields := strings.Fields(requestLine)

	if len(fields) < 3 {
		writeStatusLine(conn, 400, "Bad Request")
		endHeaders(conn)
		os.Exit(1)
	}

	method := strings.TrimSpace(fields[0])
	path := strings.TrimSpace(fields[1])
	protocol := strings.TrimSpace(fields[2])

	return Status{method, path, protocol}
}

func extractFile(fileName string) (string, error) {
	directory := os.Args[2]

	fileData, err := os.ReadFile(directory + fileName)

	if err != nil {
		return "", err
	}

	return string(fileData), nil
}

func writeFile(fileName string, body string) error {
	directory := os.Args[2]

	file, err := os.Create(directory + fileName)
	if err != nil {
		return err
	}

	_, err = file.WriteString(body)
	if err != nil {
		return err
	}

	err = file.Close()
	if err != nil {
		return err
	}

	return nil
}
