NAME="ppanel-server"
BINDIR=bin
# Keep the injected version free of the tag's leading "v"; the release pipelines
# strip it too, and cmd/run.go prints its own "v" prefix.
VERSION=$(shell git describe --tags 2>/dev/null | sed 's/^v//')
ifeq ($(strip $(VERSION)),)
VERSION=unknown version
endif
CHANNEL?=dev
BUILDTIME=$(shell date -u)
GOBUILD=CGO_ENABLED=0 go build -trimpath -ldflags '-X "github.com/perfect-panel/server/internal/app/buildinfo.Version=$(VERSION)" \
		-X "github.com/perfect-panel/server/internal/app/buildinfo.BuildTime=$(BUILDTIME)" \
		-X "github.com/perfect-panel/server/internal/app/buildinfo.Channel=$(CHANNEL)" \
		-w -s -buildid='

PLATFORM_LIST = \
	darwin-amd64 \
	darwin-amd64-v3 \
	darwin-arm64 \
	linux-386 \
	linux-amd64 \
	linux-amd64-v3 \
	linux-armv5 \
	linux-armv6 \
	linux-armv7 \
	linux-arm64 \

WINDOWS_ARCH_LIST = \
	windows-386 \
	windows-amd64 \
	windows-amd64-v3 \
	windows-arm64 \
	windows-armv7

all: linux-amd64 darwin-amd64 windows-amd64 # Most used

perf:
	bash scripts/perf/bench.sh

darwin-amd64:
	GOARCH=amd64 GOOS=darwin $(GOBUILD) -o $(BINDIR)/$(NAME)-$@

darwin-amd64-v3:
	GOARCH=amd64 GOOS=darwin GOAMD64=v3 $(GOBUILD) -o $(BINDIR)/$(NAME)-$@

darwin-arm64:
	GOARCH=arm64 GOOS=darwin $(GOBUILD) -o $(BINDIR)/$(NAME)-$@

linux-386:
	GOARCH=386 GOOS=linux $(GOBUILD) -o $(BINDIR)/$(NAME)-$@

linux-amd64:
	GOARCH=amd64 GOOS=linux $(GOBUILD) -o $(BINDIR)/$(NAME)-$@

linux-amd64-v3:
	GOARCH=amd64 GOOS=linux GOAMD64=v3 $(GOBUILD) -o $(BINDIR)/$(NAME)-$@

linux-armv5:
	GOARCH=arm GOOS=linux GOARM=5 $(GOBUILD) -o $(BINDIR)/$(NAME)-$@

linux-armv6:
	GOARCH=arm GOOS=linux GOARM=6 $(GOBUILD) -o $(BINDIR)/$(NAME)-$@

linux-armv7:
	GOARCH=arm GOOS=linux GOARM=7 $(GOBUILD) -o $(BINDIR)/$(NAME)-$@

linux-arm64:
	GOARCH=arm64 GOOS=linux $(GOBUILD) -o $(BINDIR)/$(NAME)-$@

windows-386:
	GOARCH=386 GOOS=windows $(GOBUILD) -o $(BINDIR)/$(NAME)-$@.exe

windows-amd64:
	GOARCH=amd64 GOOS=windows $(GOBUILD) -o $(BINDIR)/$(NAME)-$@.exe

windows-amd64-v3:
	GOARCH=amd64 GOOS=windows GOAMD64=v3 $(GOBUILD) -o $(BINDIR)/$(NAME)-$@.exe

windows-arm64:
	GOARCH=arm64 GOOS=windows $(GOBUILD) -o $(BINDIR)/$(NAME)-$@.exe

windows-armv7:
	GOARCH=arm GOOS=windows GOARM=7 $(GOBUILD) -o $(BINDIR)/$(NAME)-$@.exe


gz_releases=$(addsuffix .gz, $(PLATFORM_LIST))
zip_releases=$(addsuffix .zip, $(WINDOWS_ARCH_LIST))

$(gz_releases): %.gz : %
	chmod +x $(BINDIR)/$(NAME)-$(basename $@)
	gzip -f -S -$(VERSION).gz $(BINDIR)/$(NAME)-$(basename $@)

$(zip_releases): %.zip : %
	zip -m -j $(BINDIR)/$(NAME)-$(basename $@)-$(VERSION).zip $(BINDIR)/$(NAME)-$(basename $@).exe

# 【版本必须钉死】api/server/v1 的生成产物是用 protoc-gen-go v1.36.11 出的，
# 换个版本重新生成会得到风格不同的一大片 diff，甚至不兼容的代码。
# 校验方式：不改 .proto 直接跑 make proto，git diff 应当只有 protoc 版本注释那一行。
PROTOC_GEN_GO_VERSION ?= v1.36.11

.PHONY: tools proto

## tools: 安装生成代码所需的工具
tools:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VERSION)
	@command -v protoc >/dev/null || { \
		echo "还需要 protoc：macOS 用 brew install protobuf，Debian 系用 apt-get install -y protobuf-compiler"; \
		exit 1; }

## proto: 重新生成 api/server/v1 下的 pb.go
##
## 【和 ppanel-node 的 proto 必须同步】两个仓库各存一份 PushServerStatusRequest
## 等消息，字段号一一对应才能保持 wire 兼容（见 .proto 顶部注释）。
## 改了一边一定要同样改另一边，并两边都重新生成。
proto: tools
	protoc --go_out=. --go_opt=paths=source_relative -I. api/server/v1/*.proto
