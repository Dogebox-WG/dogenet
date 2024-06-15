
// avatar 48x48 Y=8 Cb=8 Cr=8 [3456] = Y [2304] Cb [576] Cr [576] = 3.375 K (3K + 384)
// avatar 48x48 Y=6 Cb=6 Cr=6 [2592] = Y [1728] Cb [432] Cr [432] = 2.53 K  (2K + 544)
// avatar 48x48 Y=5 Cb=5 Cr=5 [2160] = Y [1440] Cb [360] Cr [360] = 2.11 K  (2K + 112)
// avatar 48x48 Y=5 Cb=4 Cr=4 [2016] = Y [1440] Cb [288] Cr [288] = 1.98 K  (-32)
// avatar 48x48 Y=4 Cb=4 Cr=4 [1728] = Y [1152] Cb [288] Cr [288] = 1.68 K  (+704)
// avatar 48x48 Y=4 Cb=4 Cr=4 [1728] = Y [1152] Cb [288] Cr [288] = 1.68 K  (+704)
// avatar 48x48 12bit (444) = [3456] = 3.375 K
// avatar 48x48 16bit (565) = [4608] = 4.5 K
// display name [32]
// byline [128]

// size (3456 + 32 + 128 + 104) / 1024 = 3.6 K
// size (3456 + 32 + 128 + 104) / 1280 = 2.9 MTU

// Y a b c
// d e f g
// h i j k
// l m n o

// Y a-Y b-a c-b
// d e-d f-e g-f

// context = canvas.getContext("2d", { colorSpace: "srgb" });
// context.getImageData(0, 0, w, h, { colorSpace: "srgb" });
// drawImage(imgObj, 0, 0, imgObj.width * 0.3, imgObj.height * 0.3)
// scale and crop -> 48x48 -> color quantization -> 2016 bytes

// avatar 48x48 Y=4 Cb=4 Cr=4 [1728] = Y [1152] Cb [288] Cr [288] = 1.68 K  (+704)
// diagonal 64x64 2x2 Y=6 Cb=4 Cr=4 (32*32*6*2/8=1536) + (32*32*8/8=1024) + (32*32*2/8=256) = 2816 (2.75 K)
// 2-bit pattern: 0=| 1=/ 2=\ 3=-
// diagonal 64x64 div4 Y=6 Cb=4 Cr=4 (16*16*6*2/8=384) + (16*16*8/8=256) + (16*16*2/8=64) = 704 (0.69 K)
// 2-bit mode: 0=flat 1=linear 2=cubic
// 3-bit effect: 0=none 1=venetian 2=diagonal-LTR 3=diagonal-RTL 4=vertical 5=dots 6=splats 7=scanline-fx
// diagonal 48x48 2x2 Y=6 Cb=4 Cr=4 (24*24*6*2/8=864) + (24*24*8/8=576) + (24*24*2/8=144) = 1584 (1.54 K +48)

2x2:
*/  \*  *-  *|
/*  *\  -*  |*
