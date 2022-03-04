var http = require('http');
http.createServer(function (request, response) {
   response.writeHead(200, {'Content-Type': 'text/plain'});
   response.end('Hello World\n');
}).listen(process.env.PORT);

function logger() {
    console.log('Console output from test-node-app');
}

setInterval(logger, 1000);
