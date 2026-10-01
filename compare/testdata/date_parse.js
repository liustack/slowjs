/*
 * Javascript Micro benchmark
 *
 * Copyright (c) 2017-2019 Fabrice Bellard
 * Copyright (c) 2017-2019 Charlie Gordon
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in
 * all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL
 * THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
 * THE SOFTWARE.
 */

var global_res; /* to be sure the code is not optimized */

function date_parse(n) {
    var x0 = 0, dx = 0;
    var j;
    for(j = 0; j < n; j++) {
        var x1 = x0 - x0 % 1000;
        var x2 = -x0;
        var x3 = -x1;
        var d0 = new Date(x0);
        var d1 = new Date(x1);
        var d2 = new Date(x2);
        var d3 = new Date(x3);
        if (Date.parse(d0.toISOString()) != x0
        ||  Date.parse(d1.toGMTString()) != x1
        ||  Date.parse(d1.toString()) != x1
        ||  Date.parse(d2.toISOString()) != x2
        ||  Date.parse(d3.toGMTString()) != x3
        ||  Date.parse(d3.toString()) != x3) {
            console.log("Date.parse error for " + x0);
            return -1;
        }
        dx = (dx * 1.1 + 1) >> 0;
        x0 = (x0 + dx) % 8.64e15;
    }
    return n * 6;
}

date_parse($N);
