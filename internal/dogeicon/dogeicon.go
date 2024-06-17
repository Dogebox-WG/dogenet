package dogeicon

import (
	"log"
)

const (
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
	scaleY      = 0.1254 // as per CbCr // was: [8,240] -> [0,31]: 237+ -> 31; 12+ -> 1 // 0.1334
	biasY       = 2.0
	unscaleY    = 8.0 // was 7.3 // [0,31] -> [14,240]
	unbiasY     = 0.0
	scaleCbCr   = 0.1254    // [0,255] -> [0,31]: 248+ -> 31; 8+ -> 1
	unscaleCbCr = 8.0       // [0,31] -> [0,248]; 31 -> 248;  1 -> 8
	biasCbCr    = 127.5 + 2 // [-123.5,131.5] -> [0.0,255.0] (+4 round-to-nearest)
	unbiasCbCr  = -127.984  // 16 -> 0
)

func Y(Y float32) uint {
	s := int((Y + biasY) * scaleY)
	if s < 0 {
		return 0
	}
	if s > 31 {
		return 31
	}
	return uint(s)
}
func unY(Y uint) float32 {
	return (float32(Y) + unbiasY) * unscaleY
}

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

Other options:

	(24*24*5*2/8=720) 2Y (24*24*5*2/8=720) CbCr (24*24*2/8=144) topo = 1584 (rebalance) ••
	(24*24*5*2/8=720) 2Y (24*24*5*2/8=720) CbCr (24*24*3/8=216) topo = 1656 (more topologies)
	(24*24*12*2/8=1728) 2x rgb4 + topo (24*24*2/8=144) = 1872 (use rgb4)
	(24*24*12*2/8=1728) 2x rgb4 + topo (24*24*3/8=216) = 1944 (more topologies)
	(48*48*6/8=1728) pixels + (64*12/8) palette = 1824 (64-colour palette)
	more topologies: 4 corners, vert & horz, cross? linear: \ and / are already crosses.
	try using Y-B,Y-R (clamped to max) instead of absolute B,R for less bits?

Rebalanced Encode:

	5 Y0 5 Y1 5 Cr 5 Cb 2 Top = 22
*/
func Compress(rgb []byte, components int, options int) (comp []byte, res []byte) {
	if options&4 != 0 {
		style := byte(options & 1)
		comp := Compress1(rgb, style, components, options)
		res := Uncompress(&comp)
		log.Printf("MODE %d", style)
		return comp[:], res[:]
	}
	// try both and use the least-difference version
	compFlat := Compress1(rgb, 0, components, options)
	compLinear := Compress1(rgb, 1, components, options)
	resFlat := Uncompress(&compFlat)
	resLinear := Uncompress(&compLinear)
	if sad(rgb, resFlat[:]) < sad(rgb, resLinear[:]) {
		log.Printf("MODE 0")
		return compFlat[:], resFlat[:]
	}
	log.Printf("MODE 1")
	return compLinear[:], resLinear[:]
}

// Sum of Absolute Difference between 48x48 images.
func sad(rgb []byte, res []byte) uint {
	var sum uint // safe up to 2369x2369 image
	for x := 0; x < 48*48*3; x += 3 {
		//sum += diff(uint(rgb[x]), uint(res[x]))
		R0 := float32(rgb[x])
		G0 := float32(rgb[x+1])
		B0 := float32(rgb[x+2])
		Y0 := R0*Kr + G0*Kg + B0*Kb
		R1 := float32(res[x])
		G1 := float32(res[x+1])
		B1 := float32(res[x+2])
		Y1 := R1*Kr + G1*Kg + B1*Kb
		sum += diff(uint(Y0), uint(Y1))
	}
	return sum
}

// func cmax(Cb0, Cr0, Cb1, Cr1 float32) (Cb, Cr float32) {
// 	if math.Abs(float64(Cb0)) > math.Abs(float64(Cb1)) && math.Abs(float64(Cr0)) > math.Abs(float64(Cr1)) {
// 		return Cb0, Cr0
// 	}
// 	return Cb1, Cr1
// }

func Compress1(rgb []byte, style byte, components int, options int) (comp [1586]byte) {
	// encode flat or linear style
	topMap := topoMap[style&1]
	// encoding state
	Yacc := uint32(0)
	Ybit := 10
	compY := 1
	comp[0] = style

	// for each 2x2 tile:
	stride := 48 * components
	for y := 0; y < 48; y += 2 {
		row := y * stride
		for x := 0; x < 48; x += 2 {
			r1 := row + (x * components)
			r2 := r1 + components
			r3 := row + stride + (x * components)
			r4 := r3 + components

			// assemble RGB tile values for topology calc
			tile := &Tile{
				uint(rgb[r1]), uint(rgb[r1+1]), uint(rgb[r1+2]), // TL
				uint(rgb[r2]), uint(rgb[r2+1]), uint(rgb[r2+2]), // TR
				uint(rgb[r3]), uint(rgb[r3+1]), uint(rgb[r3+2]), // BL
				uint(rgb[r4]), uint(rgb[r4+1]), uint(rgb[r4+2]), // BR
			}

			// 1. convert 2x2 tile to YCbCr
			R0 := float32(rgb[r1])
			G0 := float32(rgb[r1+1])
			B0 := float32(rgb[r1+2])
			Y0 := R0*Kr + G0*Kg + B0*Kb
			Cb0 := R0*CbR + G0*CbG + B0*CbB
			Cr0 := R0*CrR + G0*CrG + B0*CrB
			R1 := float32(rgb[r2])
			G1 := float32(rgb[r2+1])
			B1 := float32(rgb[r2+2])
			Y1 := R1*Kr + G1*Kg + B1*Kb
			Cb1 := R1*CbR + G1*CbG + B1*CbB
			Cr1 := R1*CrR + G1*CrG + B1*CrB
			// Cb0, Cr0 = cmax(Cb0, Cr0, Cb1, Cr1)
			R2 := float32(rgb[r3])
			G2 := float32(rgb[r3+1])
			B2 := float32(rgb[r3+2])
			Y2 := R2*Kr + G2*Kg + B2*Kb
			Cb2 := R2*CbR + G2*CbG + B2*CbB
			Cr2 := R2*CrR + G2*CrG + B2*CrB
			// Cb0, Cr0 = cmax(Cb0, Cr0, Cb1, Cr1)
			R3 := float32(rgb[r4])
			G3 := float32(rgb[r4+1])
			B3 := float32(rgb[r4+2])
			Y3 := R3*Kr + G3*Kg + B3*Kb
			Cb3 := R3*CbR + G3*CbG + B3*CbB
			Cr3 := R3*CrR + G3*CrG + B3*CrB
			// Cb0, Cr0 = cmax(Cb0, Cr0, Cb1, Cr1)
			if options&24 == 8 {
				// average 4 chroma samples
				Cb0 = (Cb0 + Cb1 + Cb2 + Cb3) / 4.0
				Cr0 = (Cr0 + Cr1 + Cr2 + Cr3) / 4.0
			} else if options&24 == 16 {
				// average 2 horizontal chroma samples
				Cb0 = (Cb0 + Cb1) / 2.0
				Cr0 = (Cr0 + Cr1) / 2.0
			} else if options&24 == 24 {
				// weighted average (using intensity as weight)
				b := 1 / (1021 - (Y0 + Y1 + Y2 + Y3))
				wx := b * ((255-Y0)*0 + (255-Y1)*1 + (255-Y2)*0 + (255-Y3)*1)
				wy := b * ((255-Y0)*0 + (255-Y1)*0 + (255-Y2)*1 + (255-Y3)*1)
				Cb0 = (Cb0 * wx * wy) + (Cb1 * (1 - wx) * wy) +
					(Cb2 * wx * (1 - wy)) + (Cb3 * (1 - wx) * (1 - wx))
				Cr0 = (Cr0 * wx * wy) + (Cr1 * (1 - wx) * wy) +
					(Cr2 * wx * (1 - wy)) + (Cr3 * (1 - wx) * (1 - wx))
			}

			// compressed CbCr values (quantized)
			CbQ := uint((Cb0 + biasCbCr) * scaleCbCr)
			CrQ := uint((Cr0 + biasCbCr) * scaleCbCr)
			// clamp out of range
			if CbQ > 31 {
				CbQ = 31
			}
			if CrQ > 31 {
				CrQ = 31
			}
			// uncompressed CbCr values
			Cb := float32(CbQ)*unscaleCbCr + unbiasCbCr
			Cr := float32(CrQ)*unscaleCbCr + unbiasCbCr
			Red := Cr * RCr
			Green := Cr*GCr + Cb*GCb
			Blue := Cb * BCb

			// compressed Y values (quantized)
			Y0Q := Y(Y0)
			Y1Q := Y(Y1)
			Y2Q := Y(Y2)
			Y3Q := Y(Y3)
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

			// 2. choose topology to minimise error
			minsad := uint(4096) // > 255*12
			topology := uint(0)
			var sads [4]uint
			for q := uint(0); q < 4; q++ {
				tmap := &topMap[q]
				Ytl := unY(Ys[tmap[0]])
				tl := diff(tile.tlR, uint(Ytl+Red)) +
					diff(tile.tlG, uint(Ytl+Green)) +
					diff(tile.tlB, uint(Ytl+Blue))
				Ytr := unY(Ys[tmap[1]])
				tr := diff(tile.trR, uint(Ytr+Red)) +
					diff(tile.trG, uint(Ytr+Green)) +
					diff(tile.trB, uint(Ytr+Blue))
				Ybl := unY(Ys[tmap[2]])
				bl := diff(tile.blR, uint(Ybl+Red)) +
					diff(tile.blG, uint(Ybl+Green)) +
					diff(tile.blB, uint(Ybl+Blue))
				Ybr := unY(Ys[tmap[3]])
				br := diff(tile.brR, uint(Ybr+Red)) +
					diff(tile.brG, uint(Ybr+Green)) +
					diff(tile.brB, uint(Ybr+Blue))
				sad := tl + tr + bl + br
				sads[q] = sad
				if sad < minsad {
					minsad = sad
					topology = q
				}
			}
			// log.Printf("SAD %v %d :: %v %v %v %v :: %v", y, x, sads[0], sads[1], sads[2], sads[3], topology)
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
			// 4. encode compressed values (22 bits)
			Yacc |= uint32(Y0bits<<17|Y1bits<<12|CbQ<<7|CrQ<<2|topology) << Ybit
			if Ybit == 10 {
				// @10 wrote 22 (store top 16)
				comp[compY] = byte(Yacc >> 24)
				comp[compY+1] = byte(Yacc >> 16)
				compY += 2
				Yacc = Yacc << 16 // keep 6 bits (move to top)
				Ybit = 4          // next 22 bits at bit 4
			} else {
				// @4,6,8 wrote 22 (store top 24)
				// @4+22+6/4; @6+22+4/2 @8+22+2/0
				comp[compY] = byte(Yacc >> 24)
				comp[compY+1] = byte(Yacc >> 16)
				comp[compY+2] = byte(Yacc >> 8)
				compY += 3
				Yacc = Yacc << 24 // keep 4,2,0 bits (move to top)
				Ybit += 2         // next bits at 6 -> 8 -> 10
			}
		}
	}
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

Requires 1586 bytes (1 style + 1584 data) plus 1 padding byte for the decoder!

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
func Uncompress(comp *[1586]byte) (rgb [6912]byte) {
	// decoding state
	Yacc := uint32(comp[1]) << 24
	compY := 2
	Ybit := 0
	linear := comp[0] & 1

	// for each 2x2 tile:
	const stride = 48 * 3
	for y := 0; y < 48; y += 2 {
		row := y * stride
		for x := 0; x < 48; x += 2 {

			// 4. decode compressed values (22 bits)
			if Ybit == 0 {
				// @8 read 16 bits + 8 = 24 (use 22 keep 2)
				Yacc |= uint32(comp[compY])<<16 | uint32(comp[compY+1])<<8
				compY += 2
				Ybit = 6 // next 22 bits at bit 6
			} else {
				// @6,4,2 read 24 bits + 2,4,6 = 26,28,30 (use 22 keep 4,6,8)
				// @6+24+2/4; @4+24+4/6; @2+24+6/8
				Yacc |= (uint32(comp[compY])<<16 | uint32(comp[compY+1])<<8 | uint32(comp[compY+2])) << Ybit
				compY += 3
				Ybit -= 2 // next 22 bits at bit 4,2,0
			}
			Y0 := uint((Yacc >> 27) & 31)
			Y1 := uint((Yacc >> 22) & 31)
			Ya := unY(Y0)
			Yb := unY(Y1)
			Cb := float32((Yacc>>17)&31)*unscaleCbCr + unbiasCbCr
			Cr := float32((Yacc>>12)&31)*unscaleCbCr + unbiasCbCr
			topology := (Yacc >> 10) & 3
			Yacc = Yacc << 22 // keep low 2,4,6,8 bits

			// 2. interpolation
			var Ytl, Ytr, Ybl, Ybr float32
			if linear != 0 {
				// linear interpolation
				switch topology {
				case 0: // '/' diagonal
					Ytl = Ya // 0
					Ybr = Yb // 3
					Ytr = unY((Y0 + Y1) >> 1)
					Ybl = Ytr
				case 1: // '\' diagonal
					Ytr = Ya // 1
					Ybl = Yb // 2
					Ytl = unY((Y0 + Y1) >> 1)
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
					Ytl = Ya // 0 0
					Ytr = Ya
					Ybl = Ya // 0 3
					Ybr = Yb
				case 1: // '\' diagonal
					Ytl = Yb // 2 1
					Ytr = Ya
					Ybl = Yb // 2 2
					Ybr = Yb
				case 2: // '-' horizontal
					Ytl = Ya // 0 0
					Ytr = Ya
					Ybl = Yb // 2 2
					Ybr = Yb
				case 3: // '|' vertical
					Ytl = Ya // 0 1
					Ytr = Yb
					Ybl = Ya // 0 1
					Ybr = Yb
				}
			}

			// 3. generate pixels
			Red := Cr * RCr
			Green := Cr*GCr + Cb*GCb
			Blue := Cb * BCb
			r1 := row + (x * 3)
			r2 := r1 + stride
			noise := float32(0.0) // float32(2 - rand.Intn(5))
			// top-left pixel
			rgb[r1] = clamp(Ytl + Red + noise)
			rgb[r1+1] = clamp(Ytl + Green + noise)
			rgb[r1+2] = clamp(Ytl + Blue + noise)
			// top-right pixel
			if true {
				rgb[r1+3] = clamp(Ytr + Red + noise)
				rgb[r1+4] = clamp(Ytr + Green + noise)
				rgb[r1+5] = clamp(Ytr + Blue + noise)
				// bottom-left pixel
				rgb[r2] = clamp(Ybl + Red + noise)
				rgb[r2+1] = clamp(Ybl + Green + noise)
				rgb[r2+2] = clamp(Ybl + Blue + noise)
				// bottom-right pixel
				rgb[r2+3] = clamp(Ybr + Red + noise)
				rgb[r2+4] = clamp(Ybr + Green + noise)
				rgb[r2+5] = clamp(Ybr + Blue + noise)
			} else {
				rgb[r1+3] = clamp(Ytl + Red)
				rgb[r1+4] = clamp(Ytl + Green)
				rgb[r1+5] = clamp(Ytl + Blue)
				// bottom-left pixel
				rgb[r2] = clamp(Ytl + Red)
				rgb[r2+1] = clamp(Ytl + Green)
				rgb[r2+2] = clamp(Ytl + Blue)
				// bottom-right pixel
				rgb[r2+3] = clamp(Ytl + Red)
				rgb[r2+4] = clamp(Ytl + Green)
				rgb[r2+5] = clamp(Ytl + Blue)
			}
		}
	}
	return
}
