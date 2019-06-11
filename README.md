# Glock

## What is it?

It's sketches of a simple distributed locking system that relies upon
DynamoDB.

## Why?

Sometimes you want distributed locks.

## How?

Well, first, you probably want to turn this into production ready code.
Probably by hiring someone who actually knows how to write good Go code.

Anyway.

### Acquire A Lock

`POST/PUT`
`http://localhost:12345/locks/<lock name>?nonce=<nonce-up-to-64-chars>`

This will acquire the lock `<lock name>`. You must provide a nonce so that
you can re-acquire the lock without waiting for its expiration.

You will receive back some JSON:

```
{
    "acquireTime": "2019-06-11T22:57:31.404462301+02:00",
    "expireTime": "2019-06-11T22:58:01.404462301+02:00",
    "fence": 13,
    "body": "some contents"
}
```

The "body" is a value you can set so that you can also resume a
process that was stopped in the middle. Probably useful for
two-phase commit or some business like that.

The "fence" is used for the commands below, to prevent competing
lock holders from doing the wrong thing.


### Heartbeat A Lock

`POST/PUT`
`http://localhost:12345/locks/<lock name>/<fence>/heartbeat`

This will extend your expiration time.  If the value of `fence` is wrong, or
if the lock has expired, you will receive an error.

### Update A Lock's Value

`POST/PUT`
`http://localhost:12345/locks/<lock name>/<fence>`

Put the value in the request body. Please use strings so that
JSON doesn't barf. There's not much checking going on here yet.
If the value of `fence` is wrong, or the lock has expired,
you will receive an error.

### Release A Lock

`DELETE`
`http://localhost:12345/locks/<lock name>/<fence>`

Idempotent.

This method will always succeed so long as there's no database
failures. If you have the right fence number, it will set the
expiration time to epoch, otherwise it will just return 200
without doing anything.
