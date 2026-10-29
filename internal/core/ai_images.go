package core

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"strings"
)

func ValidateAIImages(images []string) error {
	if len(images) > 4 {
		return fmt.Errorf("最多附加 4 张图片")
	}
	total := 0
	for _, data := range images {
		header, encoded, ok := strings.Cut(data, ",")
		formats := map[string]string{"data:image/png;base64": "png", "data:image/jpeg;base64": "jpeg", "data:image/gif;base64": "gif"}
		format := formats[header]
		if !ok || format == "" || len(encoded) > base64.StdEncoding.EncodedLen(8*1024*1024) {
			return fmt.Errorf("请使用 PNG、JPEG 或 GIF 图片，每张不超过 8 MB")
		}
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || len(decoded) > 8*1024*1024 {
			return fmt.Errorf("图片数据无效或超过 8 MB")
		}
		total += len(decoded)
		if total > 16*1024*1024 {
			return fmt.Errorf("图片合计不能超过 16 MB")
		}
		config, actual, err := image.DecodeConfig(bytes.NewReader(decoded))
		if err != nil || actual != format || config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 25000000 {
			return fmt.Errorf("图片格式无效或分辨率超过 2500 万像素")
		}
	}
	return nil
}
