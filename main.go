package main

import (
	"bufio"
	"encoding/binary"
	"io"
	"log"
	"os"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
)

func readn(reader *bufio.Reader, buf []byte, n int) {
	_, err := io.ReadFull(reader, buf[:n])
	if err != nil {
		log.Fatal(err)
	}
}

func writen(writer *bufio.Writer, buf []byte, n int) {
	outn, err := writer.Write(buf[:n])
	if err != nil {
		log.Fatal(err)
	}

	if outn != n {
		log.Fatal("Data not written.")
	}
}

func checkSig(buf []byte) {
	if buf[0] == 'I' && buf[1] == 'D' && buf[2] == '3' {
		return
	}
	log.Fatal("Invalid file signature")
}

func checkVersion(version byte) {
	if version != 3 {
		log.Fatal("Version is not 3.")
	}
}

func checkFlags(flags byte) {
	async := (flags>>7)&0x1 == 1
	hasExt := (flags>>6)&0x1 == 1
	experiment := (flags>>5)&0x1 == 1
	hasFooter := (flags>>4)&0x1 == 1
	if async || hasExt || experiment || hasFooter {
		log.Fatal("Unsupported flags.")
	}
}

func checkFrameFlags(flags byte) {
	compressed := (flags>>3)&0x1 == 1
	encrypted := (flags>>2)&0x1 == 1
	async := (flags>>1)&0x1 == 1
	if compressed || encrypted || async {
		log.Fatal("Unsupported frame flags.")
	}
}

func shouldRewrite(fid string) bool {
	return fid == "TIT2" || fid == "TALB" || fid == "TPE1"
}

func decodeSync32(data []byte) uint32 {
	var result uint32
	result |= uint32(data[0]&0x7f) << 21
	result |= uint32(data[1]&0x7f) << 14
	result |= uint32(data[2]&0x7f) << 7
	result |= uint32(data[3] & 0x7f)
	return result
}

func encodeSync32(out []byte, num uint32) {
	out[0] = byte(num>>21) & 0x7f
	out[1] = byte(num>>14) & 0x7f
	out[2] = byte(num>>7) & 0x7f
	out[3] = byte(num) & 0x7f
}

func isValidFrameId(fid string) bool {
	for _, chr := range fid {
		if !(('A' <= chr && chr <= 'Z') ||
			('0' <= chr && chr <= '9')) {
			return false
		}
	}
	return true
}

func main() {
	file, err := os.Open("../a.mp3")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	out, err := os.Create("out.mp3")
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()

	reader := bufio.NewReader(file)
	writer := bufio.NewWriter(out)

	buf := make([]byte, 4096)
	readn(reader, buf, 3)
	checkSig(buf)
	writen(writer, buf, 3)

	readn(reader, buf, 2)
	version := buf[0]
	minor := buf[1]
	_ = minor
	checkVersion(version)
	writen(writer, buf, 2)

	readn(reader, buf, 1)
	checkFlags(buf[0])
	writen(writer, buf, 1)

	readn(reader, buf, 4)
	headerLen := int(decodeSync32(buf))
	writen(writer, buf, 4)

	modifiedLen := headerLen

	for i := 0; i < headerLen; {
		// Read all frame header data
		readn(reader, buf, 10)
		fid := string(buf[:4])

		// v2.3 encodes this value simply big-endian bytes,
		// but we have to change this syncsafe encoding
		// if we support v2.4.
		// http://eleken.y-lab.org/report/other/mp3tags.shtml
		fsize := int(binary.BigEndian.Uint32(buf[4:8]))

		if !isValidFrameId(fid) {
			writen(writer, buf, 10)
			break
		}

		if shouldRewrite(fid) {
			formatFlags := buf[9]
			checkFrameFlags(formatFlags)

			fdata := make([]byte, fsize)
			readn(reader, fdata, fsize)

			flags := fdata[0]
			if flags == 0x00 {
				// May be Shift_JIS

				converted, _, err := transform.Bytes(
					japanese.ShiftJIS.NewDecoder(), fdata[1:])
				if err != nil {
					// Write as-is
					writen(writer, buf, 10)
					writen(writer, fdata, fsize)
					continue
				}

				modifiedLen += len(converted) - fsize + 1

				binary.BigEndian.PutUint32(buf[4:8], uint32(len(converted)+1))
				writen(writer, buf, 10)

				// Set UTF-8 flag
				fdata[0] = 0x03
				writen(writer, fdata, 1) // Write only flag

				writen(writer, converted, len(converted))
			} else {
				writen(writer, buf, 10)
				writen(writer, fdata, fsize)
			}
		} else {
			writen(writer, buf, 10)
			if fsize > len(buf) {
				log.Fatal("Too large frame:", fsize)
			}
			readn(reader, buf, fsize)
			writen(writer, buf, fsize)
		}

		i += 10 + fsize
	}

	_, err = io.Copy(writer, reader)
	if err != nil {
		log.Fatal(err)
	}
	writer.Flush()

	out.Seek(6, 0)
	sizeBytes := make([]byte, 4)
	encodeSync32(sizeBytes, uint32(modifiedLen))
	out.Write(sizeBytes)
}
