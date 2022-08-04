module github.com/wader/ydls

go 1.12

require (
	// bump: leaktest /github.com\/fortytw2\/leaktest v(.*)/ git:https://github.com/fortytw2/leaktest.git|^1
	// bump: leaktest command go get -d github.com/fortytw2/leaktest@v$LATEST && go mod tidy
	github.com/fortytw2/leaktest v1.3.0
	// bump: goutubedl /github.com\/wader\/goutubedl .*-(.*)/ gitrefs:https://github.com/wader/goutubedl.git|re:%refs/heads/master%|@commit|/^(.{12})/
	// bump: goutubedl command go get -d github.com/wader/goutubedl && go mod tidy
	github.com/wader/goutubedl v0.0.0-20220803160425-b4e0035d5c00
	// bump: logutils /github.com\/wader\/logutils .*-(.*)/ gitrefs:https://github.com/wader/logutils.git|re:%refs/heads/master%|@commit|/^(.{12})/
	// bump: logutils command go get -d github.com/wader/logutils && go mod tidy
	github.com/wader/logutils v0.0.0-20190904144142-6d88a3144654
	// bump: osleaktest /github.com\/wader\/osleaktest .*-(.*)/ gitrefs:https://github.com/wader/osleaktest.git|re:%refs/heads/master%|@commit|/^(.{12})/
	// bump: osleaktest command go github.com/wader/osleaktest && go mod tidy
	github.com/wader/osleaktest v0.0.0-20191111175233-f643b0fed071
	// bump: sync /golang.org\/x\/sync .*-(.*)/ gitrefs:https://github.com/golang/sync.git|re:%refs/heads/master%|@commit|/^(.{12})/
	// bump: sync command go get -d golang.org/x/sync && go mod tidy
	golang.org/x/sync v0.0.0-20220722155255-886fb9371eb4
)
