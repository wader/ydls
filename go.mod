module github.com/wader/ydls

go 1.12

require (
	// bump: leaktest /github.com\/fortytw2\/leaktest v(.*)/ git:https://github.com/fortytw2/leaktest.git|^1
	github.com/fortytw2/leaktest v1.3.0
	// bump: goutubedl /github.com\/wader\/goutubedl .*-(.*)/ gitrefs:https://github.com/wader/goutubedl.git|re:%refs/heads/master%|@commit|/^(.{12})/
	github.com/wader/goutubedl v0.0.0-20200503165620-b8a0edd6e1b3
	// bump: logutils /github.com\/wader\/logutils .*-(.*)/ gitrefs:https://github.com/wader/logutils.git|re:%refs/heads/master%|@commit|/^(.{12})/
	github.com/wader/logutils v0.0.0-20190904144142-6d88a3144654
	// bump: osleaktest /github.com\/wader\/osleaktest .*-(.*)/ gitrefs:https://github.com/wader/osleaktest.git|re:%refs/heads/master%|@commit|/^(.{12})/
	github.com/wader/osleaktest v0.0.0-20191111175233-f643b0fed071
	// bump: sync /golang.org\/x\/sync .*-(.*)/ gitrefs:https://github.com/golang/sync.git|re:%refs/heads/master%|@commit|/^(.{12})/
	golang.org/x/sync v0.0.0-20200317015054-43a5402ce75a
)
