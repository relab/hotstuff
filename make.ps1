function Build-Binary {
	param (
		$Package,
		$Output
	)
	go build -o $Output $Package
}

function Get-ProtoInclude {
	$path = go list -m -f '{{.Dir}}' 'github.com/relab/gorums'
	return $path
}

function Build-ProtoFile {
	[CmdLetBinding()]
	param (
		$SrcFile,
		$GoOut,
		$GorumsOut
	)

	if ($null -eq $SrcFile) {
		return
	}

	$protoInclude = Get-ProtoInclude

	if ($null -ne $GoOut -And -Not (Compare-FileWriteTime $GoOut $SrcFile)) {
		protoc -I. -Iinternal/proto -I"$protoInclude" --go_out=paths=source_relative:. "$SrcFile"
	}

	if ($null -ne $GorumsOut -And -Not (Compare-FileWriteTime $GorumsOut $SrcFile)) {
		protoc -I. -Iinternal/proto -I"$protoInclude" --gorums_out=paths=source_relative:. "$SrcFile"
	}
}


function Compare-FileWriteTime {
	param (
		$First,
		$Second
	)

	$firstDate = (Get-ItemProperty -Path $First -Name LastWriteTime).LastWriteTime
	$secondDate = (Get-ItemProperty -Path $Second -Name LastWriteTime).LastWriteTime

	if ($null -eq $firstDate) {
		return $false
	}

	if ([datetime]$firstDate -ge [datetime]$secondDate) {
		return $true
	}

	return $false
}

go mod download
go install "github.com/relab/gorums/cmd/protoc-gen-gorums"
go install "google.golang.org/protobuf/cmd/protoc-gen-go"

Build-ProtoFile `
	-SrcFile   internal/proto/clientpb/client.proto `
	-GoOut     internal/proto/clientpb/client.pb.go `
	-GorumsOut internal/proto/clientpb/client_gorums.pb.go

Build-ProtoFile `
	-SrcFile   internal/proto/hotstuffpb/hotstuff.proto `
	-GoOut     internal/proto/hotstuffpb/hotstuff.pb.go `
	-GorumsOut internal/proto/hotstuffpb/hotstuff_gorums.pb.go

Build-ProtoFile `
	-SrcFile   internal/proto/orchestrationpb/orchestration.proto `
	-GoOut     internal/proto/orchestrationpb/orchestration.pb.go

Build-ProtoFile `
	-SrcFile   internal/proto/handelpb/handel.proto `
	-GoOut     internal/proto/handelpb/handel.pb.go `
	-GorumsOut internal/proto/handelpb/handel_gorums.pb.go

Build-ProtoFile `
	-SrcFile   metrics/types/types.proto `
	-GoOut     metrics/types/types.pb.go

Build-Binary ./cmd/hotstuff ./hotstuff.exe
Build-Binary ./cmd/plot ./plot.exe
