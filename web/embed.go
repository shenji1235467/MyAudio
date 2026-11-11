// Package web 通过 embed 将前端静态资源编译进二进制。
package web

import "embed"

// Assets 嵌入式前端资源（index.html / login.html / app.js / style.css / i18n.js / locales）
//
//go:embed index.html login.html app.js style.css i18n.js locales
var Assets embed.FS
