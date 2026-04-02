module iznik-server-go

go 1.23.0

toolchain go1.23.12

require (
	github.com/aws/aws-lambda-go v1.47.0
	github.com/awslabs/aws-lambda-go-api-proxy v0.16.2
	github.com/freegle/iznik-server-go v0.0.0-20240913084341-16eb75871cbb
	github.com/getsentry/sentry-go v0.29.0
	github.com/go-sql-driver/mysql v1.8.1
	github.com/gofiber/fiber/v2 v2.52.5
	github.com/gofiber/utils v1.1.0
	github.com/golang-jwt/jwt/v4 v4.5.0
	github.com/kellydunn/golang-geo v0.7.0
	github.com/rocketlaunchr/mysql-go v1.1.3
	github.com/stretchr/testify v1.10.0
	github.com/stripe/stripe-go/v82 v82.5.1
	github.com/tidwall/geodesic v0.3.5
	github.com/valyala/fasthttp v1.55.0
	golang.org/x/crypto v0.33.0
	gorm.io/driver/mysql v1.5.7
	gorm.io/gorm v1.31.0
	mvdan.cc/xurls/v2 v2.5.0
)

require (
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/andybalholm/brotli v1.1.0 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/erikstmartin/go-testdb v0.0.0-20160219214506-8d10e4a1bae5 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/klauspost/compress v1.17.9 // indirect
	github.com/kr/pretty v0.2.1 // indirect
	github.com/kylelemons/go-gypsy v1.0.0 // indirect
	github.com/lib/pq v1.10.9 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.15 // indirect
	github.com/ory/dockertest/v3 v3.12.0 // indirect
	github.com/philhofer/fwd v1.1.2 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/rivo/uniseg v0.4.4 // indirect
	github.com/tinylib/msgp v1.1.8 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/tcplisten v1.0.0 // indirect
	github.com/ziutek/mymysql v1.5.4 // indirect
	golang.org/x/sync v0.11.0 // indirect
	golang.org/x/sys v0.30.0 // indirect
	golang.org/x/text v0.22.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/freegle/iznik-server-go => ./

// Fix for dockertest v3.3.5 incompatibility with modern runc
replace github.com/ory/dockertest => github.com/ory/dockertest/v3 v3.10.0
