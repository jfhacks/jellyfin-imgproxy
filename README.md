# Jellyfin imgproxy

Add imgproxy to replace Jellyfin integrated image processing

> [!NOTE]  
> Use it myself, so some stuff is hardcoded e.g. ports. No intent to add a binary


## Why

- Fast compared to Jellyfin
- Options like formats
- nginx cache base on age or other rules (Jellyfin supports only all or nothing)
- Smaller image sizes, controllable quality (hardcoded in most clients)
- Secure - Jellyfin has no limits in place
    - Max size
    - force formats/ configure supported arguments
    - (Jellyfin `Images` endpoint is also unauthenticated available)
- Every image that can;t be found for whatever reason will forward the request to Jellyfin to get normal behavior

**Overall it feels much faster and slower clients will notice a boost in less resource usage**

## Requirement

- Use a linux host
- Image storage separated from media storage. **Disabled** `Save artwork into media folders`

## Not implemented

- Not all endpoint `Images` features 
     - Blur (never used it - nothing todo with blurhash)
     - Options like episodes unwatched (not cacheable, client uses normally UI)
- <details><summary>maxWidth/maxHeight and unsupported by imgproxy</summary>

    - Use-case - Image 1000x500 
    - `maxWidth`x`maxHeight` `200x200`
    - Results in 400x200 image like CSS contain
    - Possible Solution Middleware calc smallest side and sends it and other side `0`, so it scales from value send

    > [!NOTE]
    > jellyfin and most clients use fillWidth/fillHeight

    </details>
    - Other image API endpoints like Trickplay images (i run `ionotify` to convert all trickplay to webP images for fast responses and less storage usage)
    - Image library - i use application made for images and you need to mount the library to imgproxy

## Get started

1. Start imgproxy`docker-compose.yml`
    - Adjust to your setup
    - Configuration<br>
    Documentation https://docs.imgproxy.net/configuration/options
    ```yml
    ports:
      - 127.0.0.1:18889:18889 # Port you want/ only local or accessible from your container
    volumes: # Map your images - collections are outside metadata
      - /var/lib/jellyfin/metadata:/media/metadata:ro
      - /var/lib/jellyfin/data/collections:/media/collections:ro
    environment:
      - IMGPROXY_BIND=0.0.0.0:18889
      - IMGPROXY_MAX_SRC_RESOLUTION=3840 # Maximum resolution that can be requested
      - IMGPROXY_LOCAL_FILESYSTEM_ROOT=/media
      - IMGPROXY_WEBP_EFFORT=2 # CPU power vs size - webP is small, keep effort low
      - IMGPROXY_LOG_LEVEL=warn # comment out to debug and see all requests
      - IMGPROXY_CLIENT_KEEP_ALIVE_TIMEOUT=90 # Some limits (never needed them)
      - IMGPROXY_DOWNLOAD_TIMEOUT=10 # Some limits (never needed them)
      - IMGPROXY_REQUESTS_QUEUE_SIZE=500 # Queue limit (Maybe increase it, if pages are huge and cache is often a MISS)
      - IMGPROXY_WORKERS=12 # Parallel worker (default 2x cores)
      #- MALLOC_ARENA_MAX=2 # https://docs.imgproxy.net/memory_usage_tweaks#malloc_arena_max
    ```
    
    </details>
2. Setup middleware (to resolve paths)
    1. Compile it in go, add systemd or whatever you use
    ```bash
    go mod init main.go
    go mod tidy
    go build -o img-proxy -trimpath -ldflags="-s -w" main.go
    ```
    2. Now start it `./img-proxy`
    3. Add a systemd or so to keep it running e.g. `img-proxy.service`<br>**Adjust YOUR_PATH_TO**
    ```bash 
    [Unit]
    Description=Jellyfin Image Proxy
    After=network.target

    [Service]
    Type=simple

    ExecStart=YOUR_PATH_TO/img-proxy
    Restart=on-failure
    RestartSec=3

    NoNewPrivileges=true
    PrivateTmp=true

    [Install]
    WantedBy=multi-user.target
    ```
3. Update nginx to forward image location towards middleware
    - Add cache folder with correct permissions, so nginx can access it


## FAQ

### Benchmark

Just test it on your system. Source and target format & size are critical + every CPU and setup has different bottlenecks on Server side. Similar on clients, especially on low end (TV garbage SoC).

### Why not use `auth_digest` in nginx?

Tried it and it failed. Request went to proxy but header with image path was never accessible.

### My client is still slow

Check requested images. My guess is, it uses no size parameters at all or it bypasses the proxy. nginx config adds `x-cache-status` header, if proxy was used. `MISS` means the image was generated and `HIT` used nginx proxy cache.

### Why WebP?

It is small and fast, nearly all clients support it.

### How to clear cache?

Every new upload gets new UUID tag, so it will deliver a new image. Header or cookie could be added. To delete cached images, clear the nginx cache folder.

### How to run it on a Windows host?

No, idea never used it and no interest


## Additional documentation

- Jellyfin image API for reference<br>
https://api.jellyfin.org/#tag/Image/operation/GetItemImage
- imgproxy https://docs.imgproxy.net/