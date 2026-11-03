package images

import _ "embed"

//go:embed phone-on.png
var PngOnData []byte

//go:embed phone-off.png
var PngOffData []byte

//go:embed incoming.png
var PngIncomingData []byte

//go:embed phone-on.ico
var IcoOnData []byte

//go:embed phone-off.ico
var IcoOffData []byte
