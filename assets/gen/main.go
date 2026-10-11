// Command gen renders ../logo.svg to the Windows icon ../tramp.ico.
//
// Run it with go generate in the assets package. It needs Inkscape to render the SVG,
// either on the PATH or in its default install location. The icon is committed,
// so building TRAMP does not need Inkscape.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// sizes are the icon sizes in pixels. 20, 40 and the larger sizes are used at display scaling above 100%.
var sizes = []int{16, 20, 24, 32, 40, 48, 64, 256}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() (err error) {
	dir, err := os.MkdirTemp("", "tramp-icon")
	if err != nil {
		return err
	}
	defer func() {
		if rmErr := os.RemoveAll(dir); err == nil {
			err = rmErr
		}
	}()

	if err := render("logo.svg", dir); err != nil {
		return err
	}
	var images [][]byte
	for _, s := range sizes {
		data, err := os.ReadFile(pngPath(dir, s))
		if err != nil {
			return err
		}
		images = append(images, data)
	}
	return os.WriteFile("tramp.ico", encodeICO(images), 0o644)
}

func pngPath(dir string, size int) string {
	return filepath.Join(dir, fmt.Sprintf("%d.png", size))
}

// render exports the SVG to PNGs in all sizes, with a single Inkscape run.
func render(svg, dir string) error {
	ink, err := inkscape()
	if err != nil {
		return err
	}
	var actions []string
	for _, s := range sizes {
		actions = append(actions,
			fmt.Sprintf("export-width:%d", s), fmt.Sprintf("export-height:%d", s),
			"export-filename:"+filepath.ToSlash(pngPath(dir, s)), "export-do")
	}
	cmd := exec.Command(ink, "--actions="+strings.Join(actions, ";"), svg)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("inkscape: %w", err)
	}
	return nil
}

func inkscape() (string, error) {
	if p, err := exec.LookPath("inkscape"); err == nil {
		return p, nil
	}
	for _, p := range []string{
		`C:\Program Files\Inkscape\bin\inkscape.exe`,
		"/Applications/Inkscape.app/Contents/MacOS/inkscape",
	} {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("inkscape not found")
}

// encodeICO writes an ICO file with the PNG images in the order of sizes.
// PNG-compressed icons are supported since Windows Vista.
func encodeICO(images [][]byte) []byte {
	le := binary.LittleEndian
	var b bytes.Buffer
	b.Write(le.AppendUint16(nil, 0)) // reserved
	b.Write(le.AppendUint16(nil, 1)) // type: icon
	b.Write(le.AppendUint16(nil, uint16(len(images))))
	offset := 6 + 16*len(images)
	for i, s := range sizes {
		b.WriteByte(byte(s % 256))        // width, 0 means 256
		b.WriteByte(byte(s % 256))        // height
		b.Write([]byte{0, 0})             // no palette, reserved
		b.Write(le.AppendUint16(nil, 1))  // color planes
		b.Write(le.AppendUint16(nil, 32)) // bits per pixel
		b.Write(le.AppendUint32(nil, uint32(len(images[i]))))
		b.Write(le.AppendUint32(nil, uint32(offset)))
		offset += len(images[i])
	}
	for _, img := range images {
		b.Write(img)
	}
	return b.Bytes()
}
