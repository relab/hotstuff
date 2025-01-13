// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.2
// 	protoc        v5.29.2
// source: internal/proto/clientpb/client.proto

package clientpb

import (
	_ "github.com/relab/gorums"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// Command is the request that is sent to the HotStuff replicas with the data to
// be executed.
type Command struct {
	state          protoimpl.MessageState `protogen:"open.v1"`
	ClientID       uint32                 `protobuf:"varint,1,opt,name=ClientID,proto3" json:"ClientID,omitempty"`
	SequenceNumber uint64                 `protobuf:"varint,2,opt,name=SequenceNumber,proto3" json:"SequenceNumber,omitempty"`
	Data           []byte                 `protobuf:"bytes,3,opt,name=Data,proto3" json:"Data,omitempty"`
	unknownFields  protoimpl.UnknownFields
	sizeCache      protoimpl.SizeCache
}

func (x *Command) Reset() {
	*x = Command{}
	mi := &file_internal_proto_clientpb_client_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Command) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Command) ProtoMessage() {}

func (x *Command) ProtoReflect() protoreflect.Message {
	mi := &file_internal_proto_clientpb_client_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Command.ProtoReflect.Descriptor instead.
func (*Command) Descriptor() ([]byte, []int) {
	return file_internal_proto_clientpb_client_proto_rawDescGZIP(), []int{0}
}

func (x *Command) GetClientID() uint32 {
	if x != nil {
		return x.ClientID
	}
	return 0
}

func (x *Command) GetSequenceNumber() uint64 {
	if x != nil {
		return x.SequenceNumber
	}
	return 0
}

func (x *Command) GetData() []byte {
	if x != nil {
		return x.Data
	}
	return nil
}

// Batch is a list of commands to be executed
type Batch struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Commands      []*Command             `protobuf:"bytes,1,rep,name=Commands,proto3" json:"Commands,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Batch) Reset() {
	*x = Batch{}
	mi := &file_internal_proto_clientpb_client_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Batch) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Batch) ProtoMessage() {}

func (x *Batch) ProtoReflect() protoreflect.Message {
	mi := &file_internal_proto_clientpb_client_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Batch.ProtoReflect.Descriptor instead.
func (*Batch) Descriptor() ([]byte, []int) {
	return file_internal_proto_clientpb_client_proto_rawDescGZIP(), []int{1}
}

func (x *Batch) GetCommands() []*Command {
	if x != nil {
		return x.Commands
	}
	return nil
}

var File_internal_proto_clientpb_client_proto protoreflect.FileDescriptor

var file_internal_proto_clientpb_client_proto_rawDesc = []byte{
	0x0a, 0x24, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x2f, 0x63, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x70, 0x62, 0x2f, 0x63, 0x6c, 0x69, 0x65, 0x6e, 0x74,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x08, 0x63, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x70, 0x62,
	0x1a, 0x0c, 0x67, 0x6f, 0x72, 0x75, 0x6d, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x1b,
	0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f,
	0x65, 0x6d, 0x70, 0x74, 0x79, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0x61, 0x0a, 0x07, 0x43,
	0x6f, 0x6d, 0x6d, 0x61, 0x6e, 0x64, 0x12, 0x1a, 0x0a, 0x08, 0x43, 0x6c, 0x69, 0x65, 0x6e, 0x74,
	0x49, 0x44, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x08, 0x43, 0x6c, 0x69, 0x65, 0x6e, 0x74,
	0x49, 0x44, 0x12, 0x26, 0x0a, 0x0e, 0x53, 0x65, 0x71, 0x75, 0x65, 0x6e, 0x63, 0x65, 0x4e, 0x75,
	0x6d, 0x62, 0x65, 0x72, 0x18, 0x02, 0x20, 0x01, 0x28, 0x04, 0x52, 0x0e, 0x53, 0x65, 0x71, 0x75,
	0x65, 0x6e, 0x63, 0x65, 0x4e, 0x75, 0x6d, 0x62, 0x65, 0x72, 0x12, 0x12, 0x0a, 0x04, 0x44, 0x61,
	0x74, 0x61, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x04, 0x44, 0x61, 0x74, 0x61, 0x22, 0x36,
	0x0a, 0x05, 0x42, 0x61, 0x74, 0x63, 0x68, 0x12, 0x2d, 0x0a, 0x08, 0x43, 0x6f, 0x6d, 0x6d, 0x61,
	0x6e, 0x64, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x11, 0x2e, 0x63, 0x6c, 0x69, 0x65,
	0x6e, 0x74, 0x70, 0x62, 0x2e, 0x43, 0x6f, 0x6d, 0x6d, 0x61, 0x6e, 0x64, 0x52, 0x08, 0x43, 0x6f,
	0x6d, 0x6d, 0x61, 0x6e, 0x64, 0x73, 0x32, 0x4c, 0x0a, 0x06, 0x43, 0x6c, 0x69, 0x65, 0x6e, 0x74,
	0x12, 0x42, 0x0a, 0x0b, 0x45, 0x78, 0x65, 0x63, 0x43, 0x6f, 0x6d, 0x6d, 0x61, 0x6e, 0x64, 0x12,
	0x11, 0x2e, 0x63, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x70, 0x62, 0x2e, 0x43, 0x6f, 0x6d, 0x6d, 0x61,
	0x6e, 0x64, 0x1a, 0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x62, 0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x22, 0x08, 0xa0, 0xb5, 0x18, 0x01,
	0xd0, 0xb5, 0x18, 0x01, 0x42, 0x33, 0x5a, 0x31, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63,
	0x6f, 0x6d, 0x2f, 0x72, 0x65, 0x6c, 0x61, 0x62, 0x2f, 0x68, 0x6f, 0x74, 0x73, 0x74, 0x75, 0x66,
	0x66, 0x2f, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x2f, 0x63, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x70, 0x62, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x33,
}

var (
	file_internal_proto_clientpb_client_proto_rawDescOnce sync.Once
	file_internal_proto_clientpb_client_proto_rawDescData = file_internal_proto_clientpb_client_proto_rawDesc
)

func file_internal_proto_clientpb_client_proto_rawDescGZIP() []byte {
	file_internal_proto_clientpb_client_proto_rawDescOnce.Do(func() {
		file_internal_proto_clientpb_client_proto_rawDescData = protoimpl.X.CompressGZIP(file_internal_proto_clientpb_client_proto_rawDescData)
	})
	return file_internal_proto_clientpb_client_proto_rawDescData
}

var file_internal_proto_clientpb_client_proto_msgTypes = make([]protoimpl.MessageInfo, 2)
var file_internal_proto_clientpb_client_proto_goTypes = []any{
	(*Command)(nil),       // 0: clientpb.Command
	(*Batch)(nil),         // 1: clientpb.Batch
	(*emptypb.Empty)(nil), // 2: google.protobuf.Empty
}
var file_internal_proto_clientpb_client_proto_depIdxs = []int32{
	0, // 0: clientpb.Batch.Commands:type_name -> clientpb.Command
	0, // 1: clientpb.Client.ExecCommand:input_type -> clientpb.Command
	2, // 2: clientpb.Client.ExecCommand:output_type -> google.protobuf.Empty
	2, // [2:3] is the sub-list for method output_type
	1, // [1:2] is the sub-list for method input_type
	1, // [1:1] is the sub-list for extension type_name
	1, // [1:1] is the sub-list for extension extendee
	0, // [0:1] is the sub-list for field type_name
}

func init() { file_internal_proto_clientpb_client_proto_init() }
func file_internal_proto_clientpb_client_proto_init() {
	if File_internal_proto_clientpb_client_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_internal_proto_clientpb_client_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   2,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_internal_proto_clientpb_client_proto_goTypes,
		DependencyIndexes: file_internal_proto_clientpb_client_proto_depIdxs,
		MessageInfos:      file_internal_proto_clientpb_client_proto_msgTypes,
	}.Build()
	File_internal_proto_clientpb_client_proto = out.File
	file_internal_proto_clientpb_client_proto_rawDesc = nil
	file_internal_proto_clientpb_client_proto_goTypes = nil
	file_internal_proto_clientpb_client_proto_depIdxs = nil
}
