/*
Package amp provides functions for working with the AMP (Accelerated Mobile
Pages) subset of HTML, and conveying binary data through an AMP cache.

AMP cache

The CacheURL function takes a plain URL and converts it to be accessed through a
given AMP cache.

The EncodePath and DecodePath functions provide a way to encode data into the
suffix of a URL path. AMP caches do not support HTTP POST, but encoding data
into a URL path with GET is an alternative means of sending data to the server.
The format of an encoded path is:
	0<0 or more bytes, including slash>/<base64 of data>
That is:
* "0", a format version number, which controls the interpretation of the rest of
the path. Only the first byte matters as a version indicator (not the whole
first path component).
* Any number of slash or non-slash bytes. These may be used as padding or to
prevent cache collisions in the AMP cache.
* A final slash.
* base64 encoding of the data, using the URL-safe alphabet (which does not
include slash).

For example, an encoding of the string "This is path-encoded data." is the
following. The "lgWHcwhXFjUm" following the format version number is random
padding that will be ignored on decoding.
	0lgWHcwhXFjUm/VGhpcyBpcyBwYXRoLWVuY29kZWQgZGF0YS4

It is the caller's responsibility to add or remove any directory path prefix
before calling EncodePath or DecodePath.

AMP armor

AMP armor is a data encoding scheme that that satisfies the requirements of the
AMP (Accelerated Mobile Pages) subset of HTML, and survives modification by an
AMP cache. For the requirements of AMP HTML, see
https://amp.dev/documentation/guides-and-tutorials/learn/spec/amphtml/.
For modifications that may be made by an AMP cache, see
https://github.com/ampproject/amphtml/blob/main/docs/spec/amp-cache-modifications.md.

The encoding is based on ones created by Ivan Markin. See codec/amp/ in
https://github.com/nogoegst/amper and discussion at
https://bugs.torproject.org/tpo/anti-censorship/pluggable-transports/snowflake/25985.

The encoding algorithm works as follows. Base64-encode the input. Prepend the
input with the byte '0'; this is a protocol version indicator that the decoder
can use to determine how to interpret the bytes that follow. Split the base64
into fixed-size chunks separated by whitespace. Take up to 1024 chunks at a
time, and wrap them in a pre element. Then, situate the markup so far within the
body of the AMP HTML boilerplate. The decoding algorithm is to scan the HTML for
pre elements, split their text contents on whitespace and concatenate, then
base64 decode. The base64 encoding uses the standard alphabet, with normal "="
padding (https://tools.ietf.org/html/rfc4648#section-4).

The reason for splitting the base64 into chunks is that AMP caches reportedly
truncate long strings that are not broken by whitespace:
https://bugs.torproject.org/tpo/anti-censorship/pluggable-transports/snowflake/25985#note_2592348.
The characters that may separate the chunks are the ASCII whitespace characters
(https://infra.spec.whatwg.org/#ascii-whitespace) "\x09", "\x0a", "\x0c",
"\x0d", and "\x20". The reason for separating the chunks into pre elements is to
limit the amount of text a decoder may have to buffer while parsing the HTML.
Each pre element may contain at most 64 KB of text. pre elements may not be
nested.

Example

The following is the result of encoding the string
"This was encoded with AMP armor.":

	<!doctype html>
	<html amp>
	<head>
	<meta charset="utf-8">
	<script async src="https://cdn.ampproject.org/v0.js"></script>
	<link rel="canonical" href="#">
	<meta name="viewport" content="width=device-width">
	<style amp-boilerplate>body{-webkit-animation:-amp-start 8s steps(1,end) 0s 1 normal both;-moz-animation:-amp-start 8s steps(1,end) 0s 1 normal both;-ms-animation:-amp-start 8s steps(1,end) 0s 1 normal both;animation:-amp-start 8s steps(1,end) 0s 1 normal both}@-webkit-keyframes -amp-start{from{visibility:hidden}to{visibility:visible}}@-moz-keyframes -amp-start{from{visibility:hidden}to{visibility:visible}}@-ms-keyframes -amp-start{from{visibility:hidden}to{visibility:visible}}@-o-keyframes -amp-start{from{visibility:hidden}to{visibility:visible}}@keyframes -amp-start{from{visibility:hidden}to{visibility:visible}}</style><noscript><style amp-boilerplate>body{-webkit-animation:none;-moz-animation:none;-ms-animation:none;animation:none}</style></noscript>
	</head>
	<body>
	<pre>
	0VGhpcyB3YXMgZW5jb2RlZCB3aXRoIEF
	NUCBhcm1vci4=
	</pre>
	</body>
	</html>
*/
package amp
