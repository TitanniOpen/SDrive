package desktop

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const exitingSafely = "Exiting safely…"

var exitIconFrames = func() (frames [8][]byte) {
	points := [...]image.Point{{16, 5}, {24, 8}, {27, 16}, {24, 24}, {16, 27}, {8, 24}, {5, 16}, {8, 8}}
	for frame := range frames {
		img := image.NewRGBA(image.Rect(0, 0, 32, 32))
		for i, point := range points {
			distance := (frame - i + len(points)) % len(points)
			fill := color.NRGBA{65, 150, 230, uint8(255 - distance*28)}
			for y := -3; y <= 3; y++ {
				for x := -3; x <= 3; x++ {
					if x*x+y*y <= 10 {
						img.Set(point.X+x, point.Y+y, fill)
					}
				}
			}
		}
		frames[frame] = encodeTrayIcon(img)
	}
	return
}()

func (d *desktop) animateExitIcon(frame int) {
	d.tray.SetIcon(exitIconFrames[frame])
	d.iconState = ""
	var timer *time.Timer
	timer = time.AfterFunc(125*time.Millisecond, func() {
		application.InvokeAsync(func() {
			if d.stopped.Load() || d.exitSpinner != timer {
				return
			}
			if _, exiting := d.actions.Load("exit"); exiting {
				d.animateExitIcon((frame + 1) % len(exitIconFrames))
			}
		})
	})
	d.exitSpinner = timer
}

func (d *desktop) stopExitIcon() {
	if d.exitSpinner != nil {
		d.exitSpinner.Stop()
		d.exitSpinner = nil
	}
}

func trayIcon() []byte {
	return trayIconColor(color.RGBA{128, 140, 145, 255})
}

func trayIconColor(fill color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 3; y < 29; y++ {
		for x := 3; x < 29; x++ {
			img.Set(x, y, fill)
		}
	}
	for y := 9; y < 23; y++ {
		for x := 8; x < 24; x++ {
			if y < 12 || y > 19 || x < 11 || x > 20 {
				img.Set(x, y, color.RGBA{11, 30, 37, 255})
			}
		}
	}
	return encodeTrayIcon(img)
}

func encodeTrayIcon(img image.Image) []byte {
	var out bytes.Buffer
	_ = png.Encode(&out, img)
	return out.Bytes()
}
