package dogeicon

import (
	"log"
)

const (
	stride = 64 * 3
	// encoding:
	Kr  = 0.2126
	Kg  = 0.7152
	Kb  = 0.0722
	CbR = -0.1146
	CbG = -0.3854
	CbB = 0.5
	CrR = 0.5
	CrG = -0.4542
	CrB = -0.0458
	// decoding:
	RCr = 1.5748
	GCb = -0.1873
	GCr = -0.4681
	BCb = 1.8556
	// scaling
	scaleY      = 0.25   // [0,255] -> [0,63]: 252+ -> 63; 4+ -> 1
	unscaleY    = 4.048  // [0,63] -> [0,255]
	scaleCbCr   = 0.0625 // [0,255] -> [0,15]: 240+ -> 15; 16+ -> 1
	unscaleCbCr = 17.001 // [0,15] -> [0,255]
	ofsCbCr     = 127.5  // [-127.5,127.5] -> [0.0,255.0]
)

type Tile struct {
	tlR, tlG, tlB uint
	trR, trG, trB uint
	blR, blG, blB uint
	brR, brG, brB uint
}

// YTile indices:
// 0 = Y0Q // top-left
// 1 = Y1Q // top-right
// 2 = Y2Q // bottom-left
// 3 = Y3Q // bottom-right
// 4 = (Y0Q+Y3Q)/2 // '/' diagonal lerp
// 5 = (Y1Q+Y2Q)/2 // '\' diagonal lerp
// 6 = (Y0Q+Y1Q)/2 // '-' horizontal lerp top
// 7 = (Y2Q+Y3Q)/2 // '-' horizontal lerp bottom
// 8 = (Y0Q+Y2Q)/2 // '|' vertical lerp left
// 9 = (Y1Q+Y3Q)/2 // '|' vertical lerp right
type YTile [10]uint

// Indices into YTile array:
var topoMap [2][4][4]uint = [2][4][4]uint{
	// flat interpolation:
	{
		{
			// '/' diagonal
			0, 0, // 0/
			0, 3, // /3
		},
		{
			// '\' diagonal
			2, 1, // \1
			2, 2, // 2\
		},
		{
			// '-' horizontal
			0, 0, // 00 horizontal
			2, 2, // 22 no blending
		},
		{
			// '|' vertical
			0, 1, // 01 vertical
			0, 1, // 01 no blending
		},
	},
	// linear interpolation:
	{
		{
			// '/' diagonal
			0, 4, // 0/
			4, 3, // /3 gradient
		},
		{
			// '\' diagonal
			5, 1, // \1
			2, 5, // 2\ gradient
		},
		{
			// '-' horizontal
			6, 6, // 01 horizontal
			7, 7, // 23 centre samples 0+1 & 2+3
		},
		{
			// '|' vertical
			8, 9, // 01 vertical
			8, 9, // 23 centre samples 0+2 & 1+3
		},
	},
}

/*
Compress a 48x48 sRGB-8 source image to 1584-byte Y'CbCr 4:2:0 `DogeIcon` (23% original size)

Encoded as 24x24 2x2 tiles with a 2-bit tile-topology; each tile encodes Y1 Y2 Cb Cr:
Y=6bit Cb=4bit Cr=4bit (scaled full-range; no "headroom"; using BT.709)

plane0 Y1Y2 (24*24*6*2/8=864) || plane1 CbCr (24*24*4*2/8=576) || plane2 topology (24*24*2/8=144) = 1584 (1.55K)

	0 = 0/  1 = \1  2 = 00  3 = 02   2 Y-samples per tile (6-bit samples packed into 12-bit plane)
	    /3      2\      22      02   2-bit topology as per diagram (0=/ 1=\ 2=H 3=V)

Bytes are filled with bits left to right (left=MSB right=LSB)
*/
func Compress(rgb []byte, style byte) (comp [1584]byte) {
	compFlat := Compress1(rgb, 0)
	compLinear := Compress1(rgb, 1)
	resFlat := Uncompress(compFlat[:], 0)
	resLinear := Uncompress(compFlat[:], 1)
	if sad(rgb, resFlat[:]) < sad(rgb, resLinear[:]) {
		return compFlat
	}
	return compLinear
}

// Sum of Absolute Difference between 48x48 images.
func sad(rgb []byte, res []byte) uint {
	var sum uint // safe up to 2369x2369 image
	for x := 0; x < 48*48*3; x++ {
		sum += diff(uint(rgb[x]), uint(res[x]))
	}
	return sum
}

func Compress1(rgb []byte, style byte) (comp [1584]byte) {
	// encode flat or linear style
	topMap := topoMap[style&1]
	// encoding state
	Yacc := uint(0)
	Tacc := uint(0)
	Ybit := 12
	Tbit := 6
	compY := 0
	compCC := 864
	compT := 1440

	// for each 2x2 tile:
	for y := 0; y < 48; y += 2 {
		row := y * stride
		for x := 0; x < 48; x += 2 {
			r1 := row + x
			r2 := row + stride + x

			// 1. convert 2x2 tile to YCbCr
			R0 := float32(rgb[r1])
			G0 := float32(rgb[r1+1])
			B0 := float32(rgb[r1+2])
			Y0 := R0*Kr + G0*Kg + B0*Kb
			Cb0 := R0*CbR + G0*CbG + B0*CbB
			Cr0 := R0*CrR + G0*CrG + B0*CrB
			R1 := float32(rgb[r1+3])
			G1 := float32(rgb[r1+4])
			B1 := float32(rgb[r1+5])
			Y1 := R1*Kr + G1*Kg + B1*Kb
			Cb0 += R1*CbR + G1*CbG + B1*CbB
			Cr0 += R1*CrR + G1*CrG + B1*CrB
			R2 := float32(rgb[r2])
			G2 := float32(rgb[r2+1])
			B2 := float32(rgb[r2+2])
			Y2 := R2*Kr + G2*Kg + B2*Kb
			Cb0 += R2*CbR + G2*CbG + B2*CbB
			Cr0 += R2*CrR + G2*CrG + B2*CrB
			R3 := float32(rgb[r2+3])
			G3 := float32(rgb[r2+4])
			B3 := float32(rgb[r2+5])
			Y3 := R3*Kr + G3*Kg + B3*Kb
			Cb0 += R3*CbR + G3*CbG + B3*CbB
			Cr0 += R3*CrR + G3*CrG + B3*CrB

			// compressed CbCr values (quantized)
			CbQ := uint(((Cb0 / 4.0) + ofsCbCr) * scaleCbCr)
			CrQ := uint(((Cr0 / 4.0) + ofsCbCr) * scaleCbCr)
			if CbQ > 15 || CrQ > 15 {
				log.Printf("quantized CbCr out of bounds: Cb %v Cr %v\n", CbQ, CrQ)
			}
			// clamp out of range
			if CbQ > 15 {
				CbQ = 15
			}
			if CrQ > 15 {
				CrQ = 15
			}
			// uncompressed CbCr values
			Cb := float32(CbQ)*unscaleCbCr - ofsCbCr
			Cr := float32(CrQ)*unscaleCbCr - ofsCbCr
			Red := Cr * RCr
			Green := Cr*GCr + Cb*GCb
			Blue := Cb * BCb

			// compressed Y values (quantized)
			Y0Q := uint(Y0 * scaleY)
			Y1Q := uint(Y1 * scaleY)
			Y2Q := uint(Y2 * scaleY)
			Y3Q := uint(Y3 * scaleY)
			if Y0Q > 63 || Y1Q > 63 || Y2Q > 63 || Y3Q > 63 {
				log.Printf("quantized Y out of bounds: Y0 %v Y1 %v Y2 %v Y3 %v\n", Y0Q, Y1Q, Y2Q, Y3Q)
			}
			// clamp out of range
			if Y0Q > 63 {
				Y0Q = 63
			}
			if Y1Q > 63 {
				Y1Q = 63
			}
			if Y2Q > 63 {
				Y2Q = 63
			}
			if Y3Q > 63 {
				Y3Q = 63
			}
			// uncompressed Y values
			Ys := &YTile{
				Y0Q,              // top-left
				Y1Q,              // top-right
				Y2Q,              // bottom-left
				Y3Q,              // bottom-right
				(Y0Q + Y3Q) >> 1, // '/' diagonal lerp
				(Y1Q + Y2Q) >> 1, // '\' diagonal lerp
				(Y0Q + Y1Q) >> 1, // '-' horizontal lerp
				(Y2Q + Y3Q) >> 1, // '-' horizontal lerp
				(Y0Q + Y2Q) >> 1, // '|' vertical lerp
				(Y1Q + Y3Q) >> 1, // '|' vertical lerp
			}

			// assemble RGB tile values for topology calc
			tile := &Tile{
				uint(rgb[r1]), uint(rgb[r1+1]), uint(rgb[r1+2]), // TL
				uint(rgb[r1+3]), uint(rgb[r1+4]), uint(rgb[r1+5]), // TR
				uint(rgb[r2]), uint(rgb[r2+1]), uint(rgb[r2+2]), // BL
				uint(rgb[r2+3]), uint(rgb[r2+4]), uint(rgb[r2+5]), // BR
			}

			// 2. choose topology to minimise error
			minsad := uint(4096) // > 255*12
			topology := uint(0)
			for q := uint(0); q < 4; q++ {
				tmap := &topMap[q]
				Ytl := float32(Ys[tmap[0]]) * unscaleY
				tl := diff(tile.tlR, uint(Ytl+Red)) +
					diff(tile.tlG, uint(Ytl+Green)) +
					diff(tile.tlB, uint(Ytl+Blue))
				Ytr := float32(Ys[tmap[1]]) * unscaleY
				tr := diff(tile.trR, uint(Ytr+Red)) +
					diff(tile.trG, uint(Ytr+Green)) +
					diff(tile.trB, uint(Ytr+Blue))
				Ybl := float32(Ys[tmap[2]]) * unscaleY
				bl := diff(tile.blR, uint(Ybl+Red)) +
					diff(tile.blG, uint(Ybl+Green)) +
					diff(tile.blB, uint(Ybl+Blue))
				Ybr := float32(Ys[tmap[3]]) * unscaleY
				br := diff(tile.brR, uint(Ybr+Red)) +
					diff(tile.brG, uint(Ybr+Green)) +
					diff(tile.brB, uint(Ybr+Blue))
				sad := tl + tr + bl + br
				if sad < minsad {
					minsad = sad
					topology = q
				}
			}
			// 3. compute encoded Y components
			var Y0bits, Y1bits uint
			switch topology {
			case 0:
				Y0bits = Ys[topMap[0][0]]
				Y1bits = Ys[topMap[0][3]]
			case 1:
				Y0bits = Ys[topMap[1][1]]
				Y1bits = Ys[topMap[1][2]]
			case 2:
				Y0bits = Ys[topMap[2][0]]
				Y1bits = Ys[topMap[2][2]]
			case 3:
				Y0bits = Ys[topMap[3][0]]
				Y1bits = Ys[topMap[3][1]]
			}
			// 4. encode compressed values
			// Y-pairs: 36 bytes per line (24 tiles x 12 bits; 3 bytes per 2 tiles)
			Yacc |= Y0bits<<(Ybit+6) | Y1bits<<Ybit
			Ybit -= 12
			if Ybit < 0 { // 24 bits encoded
				Ybit = 12
				comp[compY] = byte(Yacc >> 16)
				comp[compY+1] = byte(Yacc >> 8)
				comp[compY+2] = byte(Yacc)
				compY += 3
				Yacc = 0
			}

			// CbCr: 24 bytes per line (24 tiles x 8 bits)
			CCacc := CbQ<<4 | CrQ
			comp[compCC] = byte(CCacc)
			compCC++

			// topology: 6 bytes per line (24 tiles x 2 bits)
			Tacc |= topology << Tbit
			Tbit -= 2
			if Tbit < 0 { // 8 bits encoded
				comp[compT] = byte(Tacc)
				compT++
				Tacc = 0
			}
		}
	}
	// for other image sizes:
	// if Ybit != 12 { output Yacc>>16, Yacc>>8 } // odd no. tiles
	// if Tbit != 6 { output Tacc } // no. tiles not divisible by 8
	return
}

func diff(x uint, y uint) uint {
	if x < y {
		return y - x
	}
	return x - y
}

func clamp(x float32) byte {
	// can go out of range due to CbCr averaging
	y := int(x)
	if y >= 0 && y <= 255 {
		return byte(y)
	}
	if y >= 0 {
		return 255
	}
	return 0
}

/*
Uncompress a `DogeIcon` to 48x48 sRGB-8 (see Compress)

Style bits:
bits 1-0: interpolation: 0=flat 1=pixelated 2=bilinear 3=bicubic
bits 4-2: effect: 0=none 1=venetian 2=vertical 3=diagonal-ltr 4=diagonal-rtl 5=dots 6=splats 7=scanline-fx
(left=MSB right=LSB)

flat: assign pixels the closest Y-value (proportional Y in tied-pixels, or tie-break consistently)
pixelated: divide the tile into four equal-sized squares (proportional Y or tie-break off-axis diagonals?)
bilinear and bicubic: in horizontal and vertical tiles: position Y at centres of point-pairs
bilinear: calculate missing corner points bilinearly (cross-tile); then standard bilinear scaling
bicubic: calculate missing corner points bicubically (cross-tile); then standard bicubic scaling (ideally)
*/
func Uncompress(comp []byte, style byte) (rgb [6912]byte) {
	// decoding state
	Yacc := uint(0)
	Tacc := uint(0)
	Ybit := -1
	Tbit := -1
	compY := 0
	compCC := 864
	compT := 1440
	linear := style & 1

	// for each 2x2 tile:
	for y := 0; y < 48; y += 2 {
		row := y * stride
		for x := 0; x < 48; x += 2 {
			r1 := row + x
			r2 := row + stride + x

			// decode Y0, Y1, topology
			if Ybit < 0 {
				Yacc = uint(comp[compY])<<16 | uint(comp[compY+1])<<8 | uint(comp[compY+2])
				compY += 3
				Ybit = 12
			}
			if Tbit < 0 {
				Tacc = uint(comp[compT])
				compT++
				Ybit = 12
			}

			Y0 := (Yacc >> (Ybit + 6)) & 63
			Y1 := (Yacc >> Ybit) & 63
			Ybit -= 12
			Ya := float32(Y0) * unscaleY
			Yb := float32(Y1) * unscaleY

			topology := (Tacc >> Tbit) & 3
			Tbit -= 2

			var Ytl, Ytr, Ybl, Ybr float32
			if linear != 0 {
				// linear interpolation
				switch topology {
				case 0: // '/' diagonal
					Ytl = Ya // 0
					Ybr = Yb // 3
					Ytr = float32((Y0+Y1)>>1) * unscaleY
					Ybl = Ytr
				case 1: // '\' diagonal
					Ytr = Ya // 1
					Ybl = Yb // 2
					Ytl = float32((Y0+Y1)>>1) * unscaleY
					Ybr = Ytl
				case 2: // '-' horizontal
					Ytl = Ya // 0
					Ytr = Ytl
					Ybl = Yb // 2
					Ybr = Ybl
				case 3: // '|' vertical
					Ytl = Ya // 0
					Ybl = Ytl
					Ytr = Yb // 1
					Ybr = Ytr
				}
			} else {
				// flat interpolation
				switch topology {
				case 0: // '/' diagonal
					Ytl = Ya // 0
					Ybr = Yb // 3
					Ytr = Ytl
					Ybl = Ytl
				case 1: // '\' diagonal
					Ytr = Ya // 1
					Ybl = Yb // 2
					Ytl = Ybl
					Ybr = Ybl
				case 2: // '-' horizontal
					Ytl = Ya // 0
					Ytr = Ytl
					Ybl = Yb // 2
					Ybr = Ybl
				case 3: // '|' vertical
					Ytl = Ya // 0
					Ybl = Ytl
					Ytr = Yb // 1
					Ybr = Ytr
				}
			}

			// decode Cb, Cr
			CbCr := comp[compCC]
			compCC++
			Cb := float32(CbCr>>4)*unscaleCbCr - ofsCbCr
			Cr := float32(CbCr&15)*unscaleCbCr - ofsCbCr
			Red := Cr * RCr
			Green := Cr*GCr + Cb*GCb
			Blue := Cb * BCb

			// top-left pixel
			rgb[r1] = clamp(Ytl + Red)
			rgb[r1+1] = clamp(Ytl + Green)
			rgb[r1+2] = clamp(Ytl + Blue)
			// top-right pixel
			rgb[r1+3] = clamp(Ytr + Red)
			rgb[r1+4] = clamp(Ytr + Green)
			rgb[r1+5] = clamp(Ytr + Blue)
			// bottom-left pixel
			rgb[r2] = clamp(Ybl + Red)
			rgb[r2+1] = clamp(Ybl + Green)
			rgb[r2+2] = clamp(Ybl + Blue)
			// bottom-right pixel
			rgb[r2+3] = clamp(Ybr + Red)
			rgb[r2+4] = clamp(Ybr + Green)
			rgb[r2+5] = clamp(Ybr + Blue)
		}
	}
	return
}
