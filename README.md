# todepot

Upload files to a [depot](https://github.com/scriptsmith/depot) server
```
Usage: todepot [URL] [PATH]...
  -a	Include hidden files
  -k int
    	Number of simultaneous uploads (default 8)
  -q	No output
  -v	Print uploaded files
```

```
$ todepot $DEPOT_URL data/
[Files: 14 / 15] [Data: 4.65 MiB / 13.92 MiB] [->___] 33.42% 521.84 KiB p/s ETA 18s
```