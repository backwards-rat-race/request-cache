A caching server for requests to make sure you're not hitting a rate limiter, for example when hitting 3rd Party APIs.

POST <url>
{
    "url": "https://google.com"
}

And this server will cache the response for 1 minute. So you can call this API a thousand times a second, but google.com will only be hit once a minute

