"use strict";
(function(){
    var u = document.getElementById('u');
    var cm = document.getElementById('cm');
    var pre = document.getElementById('pre');
    var mt = document.getElementById('mt');

    var c = document.getElementById('c');
    var d = document.getElementById('d');
    var o = document.getElementById('o');
    var g = c.getContext("2d", { colorSpace: "srgb" });
    var dg = d.getContext("2d", { colorSpace: "srgb" });
    var og = o.getContext("2d", { colorSpace: "srgb" });
    var dpi = window.devicePixelRatio || 1;

    var size = 48;
    var border = 0;
    var scale = 4;
    var blend = 24; // 24=weighted 16=horizontal 8=average 0=top-left
    var mode = 1; // 1=linear 0=pixellated
    mt.innerText = 'L';

    var total = size + (border * 2);
    c.width = total;
    c.height = total;
    var viewsize = total * scale;
    c.style.width = viewsize + 'px';
    c.style.height = viewsize + 'px';
    c.style.imageRendering = 'pixelated';

    d.width = size
    d.height = size
    d.style.width = size + 'px';
    d.style.height = size + 'px';
    d.style.imageRendering = 'pixelated';

    o.width = size
    o.height = size
    o.style.width = size + 'px';
    o.style.height = size + 'px';
    o.style.imageRendering = 'pixelated';

    var url = "";
    var pic = null;
    var zoom = 1;
    var ox = 0;
    var oy = 0;
    var tx = 0;
    var ty = 0;
    var tid = 0;
    var drag = false;

    redraw();

    u.addEventListener('change', function(ev) {
        if (ev.target.files && ev.target.files.length === 1) {
            if (url) URL.revokeObjectURL(url);
            url = URL.createObjectURL(ev.target.files[0]);
            pre.height = size; pre.src = url; // preview hack (remove)
            var img = document.createElement('img');
            img.onload = () => {
                pic = img;
                //zoom = Math.min(size/img.width, size/img.height); // contain
                zoom = Math.max(size/img.width, size/img.height); // cover
                //zoom = zoom * 2;
                var w = pic.width * zoom, h = pic.height * zoom;
                ox = (size - w)/2;
                oy = (size - h)/2;
                console.log("zoom",zoom);
                blend=24; mode=1; mt.innerText = 'L';
                redraw();
                compress();
            };
            img.src = url;
        }
    });

    function redraw() {
        g.fillStyle = "#fff";
        g.fillRect(0, 0, total, total);
        if (pic) {
            var w = pic.width * zoom, h = pic.height * zoom; // doc size
            var x = ox, y = oy;
            if (w <= size) {
                x = (total - w)/2; // centre
            } else {
                if (x < (size+border) - w) x = (size+border) - w; // stop at right (-x)
                else if (x > border) x = border; // stop at left (+x)
            }
            if (h <= size) {
                y = (total - h)/2; // centre
            } else {
                if (y < (size+border) - h) y = (size+border) - h; // stop at bottom (-y)
                if (y > border) y = border; // stop at top (+y)
            }
            console.log("draw", x, y, w, h);
            // g.imageSmoothingDisabled = true;
            g.drawImage(pic, x, y, w, h); // compressed zoom
            dg.drawImage(pic, x-border, y-border, w, h); // compressed actual size
            og.drawImage(pic, x-border, y-border, w, h); // original
        }
        g.fillStyle = "rgba(0,0,0,0.6)";
        g.fillRect(0, 0, total, border);
        g.fillRect(0, total - border, total, border);
        g.fillRect(0, border, border, size);
        g.fillRect(total - border, border, border, size);
    }

    c.addEventListener('touchstart', function(ev) {
        console.log("touchstart");
        ev.preventDefault();
        for (var touch of ev.changedTouches) {
            // screen to doc space
            // var r = c.getBoundingClientRect();
            tx = touch.clientX - ox;
            ty = touch.clientY - ox;
            tid = touch.identifier;
            drag = true;
            break;
        }
    });

    c.addEventListener('touchmove', function(ev) {
        console.log("touchmove");
        ev.preventDefault();
        if (!drag) return;
        console.log("have tid");
        for (var touch of ev.changedTouches) {
            if (touch.identifier == tid) {
                // screen to doc space
                ox = touch.clientX + tx;
                oy = touch.clientY + ty;
                redraw();
                compress();
                break;
            }
        }
    });

    c.addEventListener('touchend', function(ev) {
        ev.preventDefault();
        if (!drag) return;
        for (var touch of ev.changedTouches) {
            if (touch.identifier == tid) {
                tid = 0;
            }
        }
    });

    c.addEventListener('touchcancel', function(ev) {
        ev.preventDefault();
        drag = false;
    });

    window.addEventListener('keydown', function(ev) {
        if (ev.key == "+" || ev.key == "=") { zoom = zoom * 1.25; redraw(); }
        if (ev.key == "-" || ev.key == "_") { zoom = zoom * 0.8; redraw(); }
        if (ev.key == "r") redraw();
        if (ev.key == "c") { blend=24; mode=1; mt.innerText = 'L'; compress(); } // weighted average
        if (ev.key == "1") { blend=8; compress(); } // average 4 chroma
        if (ev.key == "2") { blend=16; compress(); } // average 2 chroma (horizontal) -- best for "aurora"
        if (ev.key == "3") { blend=0; compress(); }  // top-left chroma sample
        if (ev.key == "l") { mode = 1; mt.innerText = 'L'; compress(); } // linear
        if (ev.key == "p") { mode = 0; mt.innerText = 'P'; compress(); } // pixellated
        if (ev.key == "o") { mode = -1; mt.innerText = 'A'; compress(); } // auto (min.sad)
    });

    cm.addEventListener('click', function(ev) {
        compress();
    });

    async function compress() {
        if (!pic) return;
        var opt = blend;
        if (mode >= 0) opt |= 4 + mode;
        var snap = og.getImageData(0, 0, 48, 48);
        console.log("compress", blend, mode);

        var res = await fetch("http://localhost:8085/compress?options="+opt, {
            method: "POST",
            mode: "same-origin",
            cache: "no-cache",
            credentials: "same-origin",
            headers: {
              "Content-Type": "application/octet-stream"
            },
            redirect: "follow",
            referrerPolicy: "same-origin",
            responseType: "arraybuffer",
            body: snap.data
        });
        console.log("latest");
        if (res == undefined) { console.log("response is undefined"); return; }
        if (!res.ok) { console.log("response is not ok"); return; }
        var p = 0;
        var buf = await res.arrayBuffer();
        if (!buf) { console.log("arrayBuffer is undefined"); return; }
        var from = new Uint8Array(buf);
        if (from.length != 6912) { console.log("Wrong size: "+from.length+" (expected 6912)"); return; }
        var to = snap.data;
        for (var i=0; i<48*48*3; i+=3) {
            to[p] = from[i];
            to[p+1] = from[i+1];
            to[p+2] = from[i+2];
            to[p+3] = 255;
            p += 4;
        }
        g.putImageData(snap, border, border);
        dg.putImageData(snap, 0, 0);
    }

})();
