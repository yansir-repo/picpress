APP_NAME := PicPress
EXEC := picpress
BIN := bin/$(EXEC)
APP := bin/$(APP_NAME).app
GO_TAGS := production,nodynamic

.PHONY: build app run test clean

build:
	go build -tags "$(GO_TAGS)" -trimpath -buildvcs=false -ldflags="-s -w" -o "$(BIN)" .

app: build
	rm -rf "$(APP)"
	mkdir -p "$(APP)/Contents/MacOS" "$(APP)/Contents/Resources"
	cp "$(BIN)" "$(APP)/Contents/MacOS/$(EXEC)"
	cp build/darwin/Info.plist "$(APP)/Contents/Info.plist"
	cp build/darwin/icons.icns "$(APP)/Contents/Resources/icons.icns"
	xattr -cr "$(APP)"
	codesign --force --deep --sign - "$(APP)"
	codesign --verify --deep --strict "$(APP)"
	@echo "Built $(APP)"

run:
	go run -tags "$(GO_TAGS)" .

test:
	go test -tags nodynamic ./...

clean:
	rm -rf bin
