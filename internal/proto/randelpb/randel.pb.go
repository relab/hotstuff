// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.27.1
// 	protoc        v3.19.4
// source: internal/proto/randelpb/randel.proto

package randelpb

import (
	_ "github.com/relab/gorums"
	hotstuffpb "github.com/relab/hotstuff/internal/proto/hotstuffpb"
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

type Request struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	NodeID uint32 `protobuf:"varint,1,opt,name=NodeID,proto3" json:"NodeID,omitempty"`
	View   uint64 `protobuf:"varint,2,opt,name=View,proto3" json:"View,omitempty"`
}

func (x *Request) Reset() {
	*x = Request{}
	if protoimpl.UnsafeEnabled {
		mi := &file_internal_proto_randelpb_randel_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Request) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Request) ProtoMessage() {}

func (x *Request) ProtoReflect() protoreflect.Message {
	mi := &file_internal_proto_randelpb_randel_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Request.ProtoReflect.Descriptor instead.
func (*Request) Descriptor() ([]byte, []int) {
	return file_internal_proto_randelpb_randel_proto_rawDescGZIP(), []int{0}
}

func (x *Request) GetNodeID() uint32 {
	if x != nil {
		return x.NodeID
	}
	return 0
}

func (x *Request) GetView() uint64 {
	if x != nil {
		return x.View
	}
	return 0
}

type RContribution struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	ID          uint32                      `protobuf:"varint,1,opt,name=ID,proto3" json:"ID,omitempty"`
	Signature   *hotstuffpb.QuorumSignature `protobuf:"bytes,2,opt,name=Signature,proto3" json:"Signature,omitempty"`
	Hash        []byte                      `protobuf:"bytes,3,opt,name=Hash,proto3" json:"Hash,omitempty"`
	FailedNodes []uint32                    `protobuf:"varint,4,rep,packed,name=failedNodes,proto3" json:"failedNodes,omitempty"`
	View        uint64                      `protobuf:"varint,5,opt,name=View,proto3" json:"View,omitempty"`
}

func (x *RContribution) Reset() {
	*x = RContribution{}
	if protoimpl.UnsafeEnabled {
		mi := &file_internal_proto_randelpb_randel_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *RContribution) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*RContribution) ProtoMessage() {}

func (x *RContribution) ProtoReflect() protoreflect.Message {
	mi := &file_internal_proto_randelpb_randel_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use RContribution.ProtoReflect.Descriptor instead.
func (*RContribution) Descriptor() ([]byte, []int) {
	return file_internal_proto_randelpb_randel_proto_rawDescGZIP(), []int{1}
}

func (x *RContribution) GetID() uint32 {
	if x != nil {
		return x.ID
	}
	return 0
}

func (x *RContribution) GetSignature() *hotstuffpb.QuorumSignature {
	if x != nil {
		return x.Signature
	}
	return nil
}

func (x *RContribution) GetHash() []byte {
	if x != nil {
		return x.Hash
	}
	return nil
}

func (x *RContribution) GetFailedNodes() []uint32 {
	if x != nil {
		return x.FailedNodes
	}
	return nil
}

func (x *RContribution) GetView() uint64 {
	if x != nil {
		return x.View
	}
	return 0
}

var File_internal_proto_randelpb_randel_proto protoreflect.FileDescriptor

var file_internal_proto_randelpb_randel_proto_rawDesc = []byte{
	0x0a, 0x24, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x2f, 0x72, 0x61, 0x6e, 0x64, 0x65, 0x6c, 0x70, 0x62, 0x2f, 0x72, 0x61, 0x6e, 0x64, 0x65, 0x6c,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x08, 0x72, 0x61, 0x6e, 0x64, 0x65, 0x6c, 0x70, 0x62,
	0x1a, 0x0c, 0x67, 0x6f, 0x72, 0x75, 0x6d, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x1b,
	0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f,
	0x65, 0x6d, 0x70, 0x74, 0x79, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x19, 0x68, 0x6f, 0x74,
	0x73, 0x74, 0x75, 0x66, 0x66, 0x70, 0x62, 0x2f, 0x68, 0x6f, 0x74, 0x73, 0x74, 0x75, 0x66, 0x66,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0x35, 0x0a, 0x07, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73,
	0x74, 0x12, 0x16, 0x0a, 0x06, 0x4e, 0x6f, 0x64, 0x65, 0x49, 0x44, 0x18, 0x01, 0x20, 0x01, 0x28,
	0x0d, 0x52, 0x06, 0x4e, 0x6f, 0x64, 0x65, 0x49, 0x44, 0x12, 0x12, 0x0a, 0x04, 0x56, 0x69, 0x65,
	0x77, 0x18, 0x02, 0x20, 0x01, 0x28, 0x04, 0x52, 0x04, 0x56, 0x69, 0x65, 0x77, 0x22, 0xa4, 0x01,
	0x0a, 0x0d, 0x52, 0x43, 0x6f, 0x6e, 0x74, 0x72, 0x69, 0x62, 0x75, 0x74, 0x69, 0x6f, 0x6e, 0x12,
	0x0e, 0x0a, 0x02, 0x49, 0x44, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x02, 0x49, 0x44, 0x12,
	0x39, 0x0a, 0x09, 0x53, 0x69, 0x67, 0x6e, 0x61, 0x74, 0x75, 0x72, 0x65, 0x18, 0x02, 0x20, 0x01,
	0x28, 0x0b, 0x32, 0x1b, 0x2e, 0x68, 0x6f, 0x74, 0x73, 0x74, 0x75, 0x66, 0x66, 0x70, 0x62, 0x2e,
	0x51, 0x75, 0x6f, 0x72, 0x75, 0x6d, 0x53, 0x69, 0x67, 0x6e, 0x61, 0x74, 0x75, 0x72, 0x65, 0x52,
	0x09, 0x53, 0x69, 0x67, 0x6e, 0x61, 0x74, 0x75, 0x72, 0x65, 0x12, 0x12, 0x0a, 0x04, 0x48, 0x61,
	0x73, 0x68, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x04, 0x48, 0x61, 0x73, 0x68, 0x12, 0x20,
	0x0a, 0x0b, 0x66, 0x61, 0x69, 0x6c, 0x65, 0x64, 0x4e, 0x6f, 0x64, 0x65, 0x73, 0x18, 0x04, 0x20,
	0x03, 0x28, 0x0d, 0x52, 0x0b, 0x66, 0x61, 0x69, 0x6c, 0x65, 0x64, 0x4e, 0x6f, 0x64, 0x65, 0x73,
	0x12, 0x12, 0x0a, 0x04, 0x56, 0x69, 0x65, 0x77, 0x18, 0x05, 0x20, 0x01, 0x28, 0x04, 0x52, 0x04,
	0x56, 0x69, 0x65, 0x77, 0x32, 0xdf, 0x01, 0x0a, 0x06, 0x52, 0x61, 0x6e, 0x64, 0x65, 0x6c, 0x12,
	0x4c, 0x0a, 0x13, 0x53, 0x65, 0x6e, 0x64, 0x41, 0x63, 0x6b, 0x6e, 0x6f, 0x77, 0x6c, 0x65, 0x64,
	0x67, 0x65, 0x6d, 0x65, 0x6e, 0x74, 0x12, 0x17, 0x2e, 0x72, 0x61, 0x6e, 0x64, 0x65, 0x6c, 0x70,
	0x62, 0x2e, 0x52, 0x43, 0x6f, 0x6e, 0x74, 0x72, 0x69, 0x62, 0x75, 0x74, 0x69, 0x6f, 0x6e, 0x1a,
	0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75,
	0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x22, 0x04, 0x98, 0xb5, 0x18, 0x01, 0x12, 0x3c, 0x0a,
	0x09, 0x53, 0x65, 0x6e, 0x64, 0x4e, 0x6f, 0x41, 0x63, 0x6b, 0x12, 0x11, 0x2e, 0x72, 0x61, 0x6e,
	0x64, 0x65, 0x6c, 0x70, 0x62, 0x2e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x16, 0x2e,
	0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e,
	0x45, 0x6d, 0x70, 0x74, 0x79, 0x22, 0x04, 0x90, 0xb5, 0x18, 0x01, 0x12, 0x49, 0x0a, 0x10, 0x53,
	0x65, 0x6e, 0x64, 0x43, 0x6f, 0x6e, 0x74, 0x72, 0x69, 0x62, 0x75, 0x74, 0x69, 0x6f, 0x6e, 0x12,
	0x17, 0x2e, 0x72, 0x61, 0x6e, 0x64, 0x65, 0x6c, 0x70, 0x62, 0x2e, 0x52, 0x43, 0x6f, 0x6e, 0x74,
	0x72, 0x69, 0x62, 0x75, 0x74, 0x69, 0x6f, 0x6e, 0x1a, 0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c,
	0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79,
	0x22, 0x04, 0x90, 0xb5, 0x18, 0x01, 0x42, 0x33, 0x5a, 0x31, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62,
	0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x72, 0x65, 0x6c, 0x61, 0x62, 0x2f, 0x68, 0x6f, 0x74, 0x73, 0x74,
	0x75, 0x66, 0x66, 0x2f, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2f, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x2f, 0x72, 0x61, 0x6e, 0x64, 0x65, 0x6c, 0x70, 0x62, 0x62, 0x06, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x33,
}

var (
	file_internal_proto_randelpb_randel_proto_rawDescOnce sync.Once
	file_internal_proto_randelpb_randel_proto_rawDescData = file_internal_proto_randelpb_randel_proto_rawDesc
)

func file_internal_proto_randelpb_randel_proto_rawDescGZIP() []byte {
	file_internal_proto_randelpb_randel_proto_rawDescOnce.Do(func() {
		file_internal_proto_randelpb_randel_proto_rawDescData = protoimpl.X.CompressGZIP(file_internal_proto_randelpb_randel_proto_rawDescData)
	})
	return file_internal_proto_randelpb_randel_proto_rawDescData
}

var file_internal_proto_randelpb_randel_proto_msgTypes = make([]protoimpl.MessageInfo, 2)
var file_internal_proto_randelpb_randel_proto_goTypes = []interface{}{
	(*Request)(nil),                    // 0: randelpb.Request
	(*RContribution)(nil),              // 1: randelpb.RContribution
	(*hotstuffpb.QuorumSignature)(nil), // 2: hotstuffpb.QuorumSignature
	(*emptypb.Empty)(nil),              // 3: google.protobuf.Empty
}
var file_internal_proto_randelpb_randel_proto_depIdxs = []int32{
	2, // 0: randelpb.RContribution.Signature:type_name -> hotstuffpb.QuorumSignature
	1, // 1: randelpb.Randel.SendAcknowledgement:input_type -> randelpb.RContribution
	0, // 2: randelpb.Randel.SendNoAck:input_type -> randelpb.Request
	1, // 3: randelpb.Randel.SendContribution:input_type -> randelpb.RContribution
	3, // 4: randelpb.Randel.SendAcknowledgement:output_type -> google.protobuf.Empty
	3, // 5: randelpb.Randel.SendNoAck:output_type -> google.protobuf.Empty
	3, // 6: randelpb.Randel.SendContribution:output_type -> google.protobuf.Empty
	4, // [4:7] is the sub-list for method output_type
	1, // [1:4] is the sub-list for method input_type
	1, // [1:1] is the sub-list for extension type_name
	1, // [1:1] is the sub-list for extension extendee
	0, // [0:1] is the sub-list for field type_name
}

func init() { file_internal_proto_randelpb_randel_proto_init() }
func file_internal_proto_randelpb_randel_proto_init() {
	if File_internal_proto_randelpb_randel_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_internal_proto_randelpb_randel_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Request); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_internal_proto_randelpb_randel_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*RContribution); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_internal_proto_randelpb_randel_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   2,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_internal_proto_randelpb_randel_proto_goTypes,
		DependencyIndexes: file_internal_proto_randelpb_randel_proto_depIdxs,
		MessageInfos:      file_internal_proto_randelpb_randel_proto_msgTypes,
	}.Build()
	File_internal_proto_randelpb_randel_proto = out.File
	file_internal_proto_randelpb_randel_proto_rawDesc = nil
	file_internal_proto_randelpb_randel_proto_goTypes = nil
	file_internal_proto_randelpb_randel_proto_depIdxs = nil
}
