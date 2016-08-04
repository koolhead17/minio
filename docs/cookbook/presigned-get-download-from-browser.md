# Using pre-signed URLs to download via the browser

Instead of using an S3-compatible client to download files, you can pre-sign a
request to download a specified file from the browser, even if its bucket is
private. The following is an annotated example of how this can be used in a
Node.js application, using [minio-js](https://github.com/minio/minio-js).

### Code

```javascript
const Minio = require('minio');

var client = new Minio({
    endPoint: 'play.minio.io',
    accessKey: 'Q3AM3UQ867SPQQA43P2F',
    secretKey: 'zuf+tfteSlswRu7BJ86wekitnifILbZam1KYY3TG'
});
```

In order to sign the request, we need to log in to the database. This
creates a new `client` instance, with which you can download and upload files,
among [much more](https://github.com/minio/minio-js/blob/master/docs/API.md).

```javascript
const http = require('http');

http.createServer((req, res) => {
```

In this example, the HTTP server will generate a link to download an image of
a house from the database. It will then redirect the user to that link.

```javascript
	client.presignedGetObject('pictures', 'house.png', (err, url) => {
		if (err) throw err;

		res.redirect(url);
	});
```

[This function](https://docs.minio.io/docs/javascript-client-api-reference#presignedGetObject)
creates the URL we can use to download `pictures/house.png`. The link will
automatically expire after 7 days — this can be adjusted using the optional
`expiry` argument.

```javascript
}).listen(8080);
```

And we listen.
