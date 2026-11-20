package desktop

import (
	"bytes"
	"image/color"
	"image/png"
	"testing"
)

func TestInitialTrayIconShowsDisconnected(t *testing.T) {
	icon, err := png.Decode(bytes.NewReader(trayIcon()))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := color.RGBAModel.Convert(icon.At(4, 4)), (color.RGBA{128, 140, 145, 255}); got != want {
		t.Fatalf("initial tray color = %v, want disconnected color %v", got, want)
	}
}
