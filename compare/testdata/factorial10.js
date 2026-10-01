/*

Code adjusted from https://github.com/ngocphuongnb/go-js-engines-benchmark/blob/master/factorial/common.go

The linked repository has no license file/information.

*/
function factorial(n) {
	return n <= 1 ? 1 : n * factorial(n - 1);
}

function factorial10(n) {
	var j;
	for(j = 0; j < n; j++) {
		var i = 0;
		
		while (i++ < 1e6) {
			factorial(10);
		}
	}
}

factorial10($N);
