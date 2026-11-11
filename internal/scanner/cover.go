package scanner

import (
	"crypto/sha1"
	"encoding/hex"
)

// coverName 以音频文件绝对路径的 SHA1 作为封面文件名（不含扩展名），
// 保证同一路径封面稳定、不同路径不会冲突。
func coverName(trackPath string) string {
	sum := sha1.Sum([]byte(trackPath))
	return hex.EncodeToString(sum[:])
}
