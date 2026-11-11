package scanner

import (
	"encoding/binary"
	"errors"
	"os"
)

// wavDuration 解析 PCM WAV 文件头，按 data 区块字节数与采样参数计算精确时长（秒）。
// 仅依赖标准库，读取文件头部即可，无需加载整个音频。
func wavDuration(path string) (float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	// RIFF 头 12 字节
	var header [12]byte
	if _, err := f.Read(header[:]); err != nil {
		return 0, err
	}
	if string(header[0:4]) != "RIFF" || string(header[8:12]) != "WAVE" {
		return 0, errors.New("not a wav file")
	}

	var sampleRate uint32
	var byteRate uint32
	var dataSize uint32
	foundData := false

	// 逐块解析 fmt / data 等 chunk
	var chunkHeader [8]byte
	for {
		n, err := f.Read(chunkHeader[:])
		if n < 8 || err != nil {
			break
		}
		chunkID := string(chunkHeader[0:4])
		chunkSize := binary.LittleEndian.Uint32(chunkHeader[4:8])

		switch chunkID {
		case "fmt ":
			buf := make([]byte, chunkSize)
			if _, err := f.Read(buf); err != nil {
				return 0, err
			}
			if len(buf) >= 16 {
				sampleRate = binary.LittleEndian.Uint32(buf[4:8])
				byteRate = binary.LittleEndian.Uint32(buf[8:12])
			}
			// chunk 大小为奇数时含 1 字节填充
			if chunkSize%2 == 1 {
				if _, err := f.Seek(1, 1); err != nil {
					return 0, err
				}
			}
		case "data":
			dataSize = chunkSize
			foundData = true
		default:
			// 跳过未知 chunk
			if _, err := f.Seek(int64(chunkSize)+int64(chunkSize%2), 1); err != nil {
				return 0, err
			}
		}
		if foundData && byteRate > 0 {
			break
		}
	}

	if !foundData || byteRate == 0 || sampleRate == 0 {
		return 0, errors.New("incomplete wav header")
	}
	return float64(dataSize) / float64(byteRate), nil
}
