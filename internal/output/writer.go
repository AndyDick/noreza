package output

import (
	"log"

	"github.com/bendahl/uinput"
	"github.com/caedis/noreza/internal/mapping"
)

type Writer struct {
	keyboard uinput.Keyboard
	mouse    *ExtMouse
}

func NewWriter(serial string) (*Writer, error) {
	kb, err := uinput.CreateKeyboard("/dev/uinput", []byte("noreza-keyboard-"+serial[len(serial)-4:]))
	if err != nil {
		return nil, err
	}
	mouse, err := CreateExtMouse("/dev/uinput", []byte("noreza-mouse-"+serial[len(serial)-4:]))
	if err != nil {
		return nil, err
	}
	return &Writer{keyboard: kb, mouse: mouse}, nil
}

func (w *Writer) Close() {
	w.keyboard.Close()
	w.mouse.Close()
}

func (w *Writer) Apply(press, release []mapping.KeyMapping) {
	for _, key := range release {
		switch key.Mode {
		case mapping.Mouse:
			switch key.Code {
			case 0x110:
				w.mouse.LeftRelease()
			case 0x112:
				w.mouse.MiddleRelease()
			case 0x111:
				w.mouse.RightRelease()
			case 0x113: // Mouse4 (BTN_SIDE)
				w.mouse.ButtonRelease(0x113)
			case 0x114: // Mouse5 (BTN_EXTRA)
				w.mouse.ButtonRelease(0x114)
			// Scroll wheel doesn't have release events
			}
		case mapping.Keyboard:
			w.keyboard.KeyUp(key.Code)
		}
	}
	for _, key := range press {
		switch key.Mode {
		case mapping.Mouse:
			switch key.Code {
			case 0x110:
				w.mouse.LeftPress()
			case 0x112:
				w.mouse.MiddlePress()
			case 0x111:
				w.mouse.RightPress()
			case 0x001:
				if err := w.mouse.Wheel(false, 2); err != nil {
					log.Printf("Scroll up error: %v", err)
				}
			case 0x002:
				if err := w.mouse.Wheel(false, -2); err != nil {
					log.Printf("Scroll down error: %v", err)
				}
			case 0x113: // Mouse4 (BTN_SIDE)
				w.mouse.ButtonPress(0x113)
			case 0x114: // Mouse5 (BTN_EXTRA)
				w.mouse.ButtonPress(0x114)
			}
		case mapping.Keyboard:
			w.keyboard.KeyDown(key.Code)
		}
	}
}
