package scanner

import (
	"encoding/binary"
	"io"
	"os"
)

// WAV/RIFF 头部相关常量（字节偏移与字段长度）。
const (
	fmtChunkOffset      = 12 // "fmt " 子块起始偏移
	fmtFieldSize        = 16 // PCM fmt 块固定 16 字节
	bitsPerSampleOffset = 22 // fmt 块内位深字段偏移
	dataSizeFieldOffset = 40 // 标准 PCM 头部中 data 块大小字段偏移
	dataPayloadOffset   = 44 // 标准 PCM 头部之后、音频采样数据的起始偏移
)

// wavDurationSec 解析 WAV 时长（秒），保留一位小数。
// 优先按标准 44 字节 PCM 头解析；头部不规整时回退为“数据字节 / 码率”估算。
func wavDurationSec(path string) float64 {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	header := make([]byte, dataPayloadOffset)
	n, err := io.ReadFull(f, header)
	header = header[:n]
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return 0
	}

	// 校验 RIFF/WAVE 与 fmt 标识，避免把非 WAV 文件误解析。
	if len(header) < fmtChunkOffset+4 ||
		string(header[0:4]) != "RIFF" || string(header[8:12]) != "WAVE" ||
		string(header[12:16]) != "fmt " {
		return 0
	}

	sampleRate := binary.LittleEndian.Uint32(header[24:28])
	channels := binary.LittleEndian.Uint16(header[22:24])
	bitsPerSample := binary.LittleEndian.Uint16(header[bitsPerSampleOffset : bitsPerSampleOffset+2])
	if sampleRate == 0 || channels == 0 || bitsPerSample == 0 {
		return 0 // 头部参数非法，无法计算，交给前端按 0 兜底
	}
	bytesPerSec := uint32(channels) * sampleRate * uint32(bitsPerSample) / 8
	if bytesPerSec == 0 {
		return 0
	}

	// 标准 PCM 头：直接用 data 块大小计算；否则用文件总长度回退估算。
	var dataBytes uint32
	if len(header) >= dataSizeFieldOffset+4 && string(header[36:40]) == "data" {
		dataBytes = binary.LittleEndian.Uint32(header[dataSizeFieldOffset : dataSizeFieldOffset+4])
	} else if fi, statErr := f.Stat(); statErr == nil && fi.Size() > fmtChunkOffset+fmtFieldSize {
		// 非标准头无法定位 data 块，按文件大小减去头部做保守估算。
		dataBytes = uint32(fi.Size() - fmtChunkOffset - fmtFieldSize)
	}
	if dataBytes == 0 {
		return 0
	}
	return float64(dataBytes) / float64(bytesPerSec)
}
