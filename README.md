# snowflake-pt

A Pluggable Transport using WebRTC

### Usage

Open up six terminals:

**client:**

```
cd client/
go build
```

1. tor -f torrc SOCKSPort auto
2. cat > signal
3. tail -F snowflake.log

**server:**

```
cd server/
go build
```

4. tor -f torrc
5. cat > signal
6. tail -F snowflake.log

Look for the offer in terminal 3; copy and paste it into terminal 5.
Copy and paste the answer in terminal 6 to terminal 2.
At this point the tor client should bootstrap to 100%.

### More

More documentation on the way.
