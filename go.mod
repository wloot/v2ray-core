module github.com/v2fly/v2ray-core/v5

go 1.22.0

require (
	github.com/adrg/xdg v0.5.0
	github.com/go-chi/chi/v5 v5.1.0
	github.com/go-chi/render v1.0.3
	github.com/go-playground/validator/v10 v10.22.0
	github.com/golang/mock v1.7.0-rc.1
	github.com/golang/protobuf v1.5.4
	github.com/google/go-cmp v0.6.0
	github.com/google/gopacket v1.1.19
	github.com/gorilla/websocket v1.5.3
	github.com/jhump/protoreflect v1.16.0
	github.com/lunixbochs/struc v0.0.0-20200707160740-784aaebc1d40
	github.com/miekg/dns v1.1.61
	github.com/mustafaturan/bus v1.0.2
	github.com/pelletier/go-toml v1.9.5
	github.com/pion/transport/v2 v2.2.8
	github.com/pires/go-proxyproto v0.7.0
	github.com/quic-go/quic-go v0.45.1
	github.com/refraction-networking/utls v1.6.7
	github.com/sagernet/sing v0.4.2
	github.com/sagernet/sing-shadowsocks v0.2.7
	github.com/seiflotfy/cuckoofilter v0.0.0-20240715131351-a2f2c23f1771
	github.com/stretchr/testify v1.9.0
	github.com/v2fly/BrowserBridge v0.0.0-20210430233438-0570fc1d7d08
	github.com/v2fly/VSign v0.0.0-20201108000810-e2adc24bf848
	github.com/v2fly/ss-bloomring v0.0.0-20210312155135-28617310f63e
	github.com/vincent-petithory/dataurl v1.0.0
	github.com/xiaokangwang/VLite v0.0.0-20231225174116-75fa4b06e9f2
	go.starlark.net v0.0.0-20240705175910-70002002b310
	go4.org/netipx v0.0.0-20231129151722-fdeea329fbba
	golang.org/x/crypto v0.25.0
	golang.org/x/net v0.27.0
	golang.org/x/sync v0.7.0
	golang.org/x/sys v0.22.0
	google.golang.org/grpc v1.65.0
	google.golang.org/protobuf v1.34.2
	gopkg.in/yaml.v3 v3.0.1
	gvisor.dev/gvisor v0.0.0-20240625000500-31fa765f9b35
	h12.io/socks v1.0.3
	lukechampine.com/blake3 v1.3.0
)

require (
	github.com/aead/cmac v0.0.0-20160719120800-7af84192f0b1 // indirect
	github.com/ajg/form v1.5.1 // indirect
	github.com/andybalholm/brotli v1.1.0 // indirect
	github.com/boljen/go-bitmap v0.0.0-20151001105940-23cd2fb0ce7d // indirect
	github.com/bufbuild/protocompile v0.14.0 // indirect
	github.com/cloudflare/circl v1.3.9 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dgryski/go-metro v0.0.0-20211217172704-adc40b04c140 // indirect
	github.com/ebfe/bcrypt_pbkdf v0.0.0-20140212075826-3c8d2dcb253a // indirect
	github.com/gabriel-vasile/mimetype v1.4.4 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/google/btree v1.1.2 // indirect
	github.com/google/pprof v0.0.0-20240722153945-304e4f0156b8 // indirect
	github.com/klauspost/compress v1.17.9 // indirect
	github.com/klauspost/cpuid/v2 v2.2.8 // indirect
	github.com/klauspost/reedsolomon v1.12.3 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/mustafaturan/monoton v1.0.0 // indirect
	github.com/onsi/ginkgo/v2 v2.19.0 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/pion/dtls/v2 v2.2.12 // indirect
	github.com/pion/logging v0.2.2 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/sctp v1.8.19 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/riobard/go-bloom v0.0.0-20200614022211-cdc8013cb5b3 // indirect
	github.com/secure-io/siv-go v0.0.0-20180922214919-5ff40651e2c4 // indirect
	github.com/xtaci/smux v1.5.24 // indirect
	go.uber.org/mock v0.4.0 // indirect
	golang.org/x/exp v0.0.0-20240719175910-8a7402abbf56 // indirect
	golang.org/x/mod v0.19.0 // indirect
	golang.org/x/text v0.16.0 // indirect
	golang.org/x/time v0.5.0 // indirect
	golang.org/x/tools v0.23.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240722135656-d784300faade // indirect
)

replace github.com/lunixbochs/struc v0.0.0-20200707160740-784aaebc1d40 => github.com/xiaokangwang/struc v0.0.0-20231031203518-0e381172f248
