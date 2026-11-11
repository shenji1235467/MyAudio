// Package web 通过 embed 将前端静态资源编译进二进制。
package web

import "embed"

// Assets 嵌入式前端资源（index.html / login.html / app.js / style.css）
//
//go:embed index.html login.html app.js style.css
var Assets embed.FS
