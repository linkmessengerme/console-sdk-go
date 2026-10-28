# Merchant contract fixtures

`payments.json` was serialized with the actual SDK gateway protobuf messages and
`protojson.MarshalOptions{UseProtoNames:true, EmitUnpopulated:false}`. Its metadata
records the source path, gateway commit and SHA-256 of the public payment proto.
It includes every merchant route, every non-success business result and the common
ProtoJSON error shape. The deliberately large int64 values test wire precision;
they do not assert that a payment grant allows such amounts. No live services are
contacted by these tests.

When the public contract changes, regenerate these snapshots from the gateway
protobuf types with the same marshal options, then review the SDK mapping tests.
