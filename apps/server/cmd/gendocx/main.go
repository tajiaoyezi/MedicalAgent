// gendocx 生成一个最小合法 .docx（OOXML zip：[Content_Types].xml + _rels/.rels + word/document.xml），
// 供 e2e 真实 ONLYOFFICE DS 渲染测试用（合成文本字节非合法 docx，DS 会报错不渲染）。
// 用法：go run ./cmd/gendocx <输出路径>
package main

import (
	"archive/zip"
	"log"
	"os"
)

const contentTypes = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`

const rels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`

const document = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>MedOffice c05 真实 ONLYOFFICE 写回测试原始正文段落。</w:t></w:r></w:p><w:sectPr/></w:body></w:document>`

func main() {
	if len(os.Args) < 2 {
		log.Fatal("用法：gendocx <输出路径>")
	}
	f, err := os.Create(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for _, e := range []struct{ name, body string }{
		{"[Content_Types].xml", contentTypes},
		{"_rels/.rels", rels},
		{"word/document.xml", document},
	} {
		w, err := zw.Create(e.name)
		if err != nil {
			log.Fatal(err)
		}
		if _, err := w.Write([]byte(e.body)); err != nil {
			log.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %s", os.Args[1])
}
