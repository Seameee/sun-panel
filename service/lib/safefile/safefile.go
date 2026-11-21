// Package safefile 收敛上传文件的类型判定与下发策略：
//   - 以内容嗅探（magic bytes）决定真实图片类型，不信任客户端文件名后缀；
//   - 图片按 inert 策略下发（nosniff；SVG 追加 CSP sandbox）；
//   - HTML/JS/CSS/XML 等主动内容一律作为附件下载，绝不在本站源内渲染。
package safefile

import (
	"encoding/xml"
	"net/http"
	"path"
	"strings"
)

const (
	// SniffLen 参与嗅探的最大字节数。
	SniffLen = 4 << 10
	// MaxImageBytes 单张图片上传上限（10 MiB）。
	MaxImageBytes = 10 << 20
	// MaxMultipartBytes 多文件上传请求体上限（64 MiB）。
	MaxMultipartBytes = 64 << 20
)

var inlineImageTypes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".jpe":  "image/jpeg",
	".jfif": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".bmp":  "image/bmp",
	".ico":  "image/x-icon",
	".svg":  "image/svg+xml",
	".avif": "image/avif",
	".heic": "image/heic",
	".heif": "image/heif",
}

var activeExts = map[string]bool{
	".html":  true,
	".htm":   true,
	".xhtml": true,
	".shtml": true,
	".js":    true,
	".mjs":   true,
	".cjs":   true,
	".css":   true,
	".xml":   true,
	".xsl":   true,
	".svgz":  true,
}

// NormalizeExt 统一小写并补前导点。
func NormalizeExt(ext string) string {
	ext = strings.ToLower(strings.TrimSpace(ext))
	if ext == "" {
		return ""
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	return ext
}

// ExtOf 从文件名取规范化扩展名。
func ExtOf(name string) string {
	return NormalizeExt(path.Ext(name))
}

// IsAllowedUploadExt 上传白名单（仅图片类）。
func IsAllowedUploadExt(ext string) bool {
	_, ok := inlineImageTypes[NormalizeExt(ext)]
	return ok
}

// IsInlineImage 是否可按图片内联下发。
func IsInlineImage(ext string) bool {
	_, ok := inlineImageTypes[NormalizeExt(ext)]
	return ok
}

// IsActiveContent 是否属于可能在本站源内执行的主动内容。
func IsActiveContent(ext string) bool {
	return activeExts[NormalizeExt(ext)]
}

// DetectImageExt 依据内容嗅探返回规范扩展名；非图片返回 ok=false。
func DetectImageExt(head []byte) (string, bool) {
	if len(head) == 0 {
		return "", false
	}

	switch http.DetectContentType(head) {
	case "image/png":
		return ".png", true
	case "image/jpeg":
		return ".jpg", true
	case "image/gif":
		return ".gif", true
	case "image/bmp":
		return ".bmp", true
	case "image/webp":
		return ".webp", true
	case "image/x-icon", "image/vnd.microsoft.icon":
		return ".ico", true
	case "image/avif":
		return ".avif", true
	case "image/heic", "image/heif":
		return ".heic", true
	}

	// 部分容器格式需要自行判断 brand / 魔数
	if len(head) >= 12 && string(head[0:4]) == "RIFF" && string(head[8:12]) == "WEBP" {
		return ".webp", true
	}
	if len(head) >= 12 && string(head[4:8]) == "ftyp" {
		brand := string(head[8:12])
		if brand == "avif" || brand == "avis" {
			return ".avif", true
		}
		if strings.HasPrefix(brand, "hei") || strings.HasPrefix(brand, "hev") || brand == "mif1" || brand == "msf1" {
			return ".heic", true
		}
	}
	if len(head) >= 4 && head[0] == 0x00 && head[1] == 0x00 && (head[2] == 0x01 || head[2] == 0x02) && head[3] == 0x00 {
		return ".ico", true
	}

	// SVG 属于文本，嗅探会得到 text/plain 或 text/xml
	if looksLikeSVG(head) {
		return ".svg", true
	}

	return "", false
}

func looksLikeSVG(head []byte) bool {
	decoder := xml.NewDecoder(strings.NewReader(strings.TrimPrefix(string(head), "\ufeff")))
	for {
		token, err := decoder.Token()
		if err != nil {
			return false
		}
		switch t := token.(type) {
		case xml.StartElement:
			return t.Name.Local == "svg" && (t.Name.Space == "" || t.Name.Space == "http://www.w3.org/2000/svg")
		case xml.CharData:
			if strings.TrimSpace(string(t)) != "" {
				return false
			}
		}
	}
}

// ContentType 返回下发用的 MIME 类型；非图片按二进制处理。
func ContentType(ext string) string {
	ext = NormalizeExt(ext)
	if ct, ok := inlineImageTypes[ext]; ok {
		return ct
	}
	return "application/octet-stream"
}

// FilenameDisposition 生成安全的 Content-Disposition。
// 中文等非 ASCII 文件名使用 RFC 5987 的 filename*。
func FilenameDisposition(disposition, name string) string {
	disposition = strings.ToLower(strings.TrimSpace(disposition))
	if disposition != "inline" && disposition != "attachment" {
		disposition = "attachment"
	}

	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, "\"", "_")
	name = strings.ReplaceAll(name, "\r", "_")
	name = strings.ReplaceAll(name, "\n", "_")

	ascii := true
	for _, r := range name {
		if r > 0x7e || r < 0x20 {
			ascii = false
			break
		}
	}
	if ascii && name != "" {
		return disposition + `; filename="` + name + `"`
	}

	fallback := name
	if fallback == "" {
		fallback = "file"
	}
	// 百分号编码（RFC 5987）
	var sb strings.Builder
	for i := 0; i < len(fallback); i++ {
		c := fallback[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '.' || c == '_' || c == '~' {
			sb.WriteByte(c)
			continue
		}
		sb.WriteByte('%')
		sb.WriteString(strings.ToUpper(strconvHex(c)))
	}
	return disposition + `; filename="` + strings.ReplaceAll(fallback, `"`, "_") + `"; filename*=UTF-8''` + sb.String()
}

func strconvHex(b byte) string {
	const hex = "0123456789abcdef"
	return string([]byte{hex[b>>4], hex[b&0x0f]})
}

// ApplyServeHeaders 为静态下发设置安全响应头。
// 图片按 inert 策略内联；主动内容与未知类型强制附件下载。
func ApplyServeHeaders(h http.Header, ext, name string) {
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; img-src 'self' data:; sandbox")

	ext = NormalizeExt(ext)
	if IsInlineImage(ext) {
		h.Set("Content-Type", ContentType(ext))
		h.Set("Content-Disposition", FilenameDisposition("inline", name))
		// SVG 可能携带脚本，额外用 sandbox 指令保证文档惰性
		if ext == ".svg" {
			h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
		}
		return
	}

	h.Set("Content-Type", "application/octet-stream")
	h.Set("Content-Disposition", FilenameDisposition("attachment", name))
}

// CanonicalExt 归一化等价后缀（.jpeg 与 .jpg 视为同类）。
func CanonicalExt(ext string) string {
	ext = NormalizeExt(ext)
	switch ext {
	case ".jpeg", ".jpe", ".jfif":
		return ".jpg"
	case ".heif":
		return ".heic"
	}
	return ext
}

// SameImageType 判断客户端后缀与嗅探结果是否指向同一类图片。
func SameImageType(clientExt, detectedExt string) bool {
	return CanonicalExt(clientExt) == CanonicalExt(detectedExt)
}
