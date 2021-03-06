package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
)

func readN(reader *bufio.Reader, buf []byte, n int) bool {
	_, err := io.ReadFull(reader, buf[:n])
	if err != nil {
		return false
	}
	return true
}

func writeN(writer *bufio.Writer, buf []byte, n int) bool {
	outn, err := writer.Write(buf[:n])
	if err != nil || outn != n {
		return false
	}
	return true
}

func checkAndWrite(reader *bufio.Reader, writer *bufio.Writer, buf []byte,
	checker func(buf []byte) bool) bool {
	if !readN(reader, buf, len(buf)) || !checker(buf) || !writeN(writer, buf, len(buf)) {
		return false
	}
	return true
}

func checkSig(buf []byte) bool {
	if buf[0] == 'I' && buf[1] == 'D' && buf[2] == '3' {
		return true
	}
	return false
}

func checkVersion(version []byte) bool {
	if version[0] != 3 {
		return false
	}
	return true
}

func checkFlags(flags []byte) bool {
	f := flags[0]
	async := (f>>7)&0x1 == 1
	hasExt := (f>>6)&0x1 == 1
	experiment := (f>>5)&0x1 == 1
	hasFooter := (f>>4)&0x1 == 1
	return !(async || hasExt || experiment || hasFooter)
}

func checkFrameFlags(flags byte) bool {
	compressed := (flags>>3)&0x1 == 1
	encrypted := (flags>>2)&0x1 == 1
	async := (flags>>1)&0x1 == 1
	return !(compressed || encrypted || async)
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

func convertFile(inPath, outPath string) bool {
	file, err := os.Open(inPath)
	if err != nil {
		return false
	}
	defer file.Close()

	out, err := os.Create(outPath)
	if err != nil {
		return false
	}
	defer out.Close()

	reader := bufio.NewReader(file)
	writer := bufio.NewWriter(out)

	buf := make([]byte, 4096)
	if !checkAndWrite(reader, writer, buf[:3], checkSig) {
		return false
	}

	if !checkAndWrite(reader, writer, buf[:2], checkVersion) {
		return false
	}

	if !checkAndWrite(reader, writer, buf[:1], checkFlags) {
		return false
	}

	if !readN(reader, buf, 4) {
		return false
	}
	headerLen := int(decodeSync32(buf))
	if !writeN(writer, buf, 4) {
		return false
	}

	modifiedLen := headerLen

	for i := 0; i < headerLen; {
		// Read all frame header data
		if !readN(reader, buf, 10) {
			return false
		}
		fid := string(buf[:4])

		// v2.3 encodes this value simply big-endian bytes,
		// but we have to change this syncsafe encoding
		// if we support v2.4.
		// http://eleken.y-lab.org/report/other/mp3tags.shtml
		fsize := int(binary.BigEndian.Uint32(buf[4:8]))

		if !isValidFrameId(fid) {
			if !writeN(writer, buf, 10) {
				return false
			}
			break
		}

		if shouldRewrite(fid) {
			formatFlags := buf[9]
			if !checkFrameFlags(formatFlags) {
				return false
			}

			fdata := make([]byte, fsize)
			if !readN(reader, fdata, fsize) {
				return false
			}

			flags := fdata[0]
			if flags == 0x00 {
				// May be Shift_JIS

				converted, _, err := transform.Bytes(
					japanese.ShiftJIS.NewDecoder(), fdata[1:])
				if err != nil {
					// Write as-is
					if !writeN(writer, buf, 10) || !writeN(writer, fdata, fsize) {
						return false
					}
					continue
				}

				modifiedLen += len(converted) - fsize + 1

				binary.BigEndian.PutUint32(buf[4:8], uint32(len(converted)+1))
				if !writeN(writer, buf, 10) {
					return false
				}

				// Set UTF-8 flag
				fdata[0] = 0x03
				// Write only flag
				if !writeN(writer, fdata, 1) {
					return false
				}

				if !writeN(writer, converted, len(converted)) {
					return false
				}
			} else {
				if !writeN(writer, buf, 10) || !writeN(writer, fdata, fsize) {
					return false
				}
			}
		} else {
			if !writeN(writer, buf, 10) {
				return false
			}

			if _, err := io.CopyN(writer, reader, int64(fsize)); err != nil {
				return false
			}
		}

		i += 10 + fsize
	}

	if _, err := io.Copy(writer, reader); err != nil {
		return false
	}
	writer.Flush()

	out.Seek(6, 0)
	sizeBytes := make([]byte, 4)
	encodeSync32(sizeBytes, uint32(modifiedLen))
	out.Write(sizeBytes)

	return true
}

func main() {
	var inputPaths []string
	for _, arg := range os.Args[1:] {
		inputs, err := getInputFiles(arg)
		if err != nil {
			log.Fatal(err)
		}
		inputPaths = append(inputPaths, inputs...)
	}

	for _, inputPath := range inputPaths {
		outputPath := getOutPath(inputPath)
		if !convertFile(inputPath, outputPath) {
			fmt.Printf("Error converting %s\n", inputPath)
			continue
		}

		if err := moveFile(outputPath, inputPath); err != nil {
			fmt.Printf("Error renaming %s: %s\n", outputPath, err)
		}
	}
}
