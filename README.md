# ydb

[![PkgGoDev](https://pkg.go.dev/badge/github.com/ydb-platform/ydb-go-sdk/v3)](https://pkg.go.dev/github.com/ydb-platform/ydb-go-sdk/v3)
[![GoDoc](https://godoc.org/github.com/ydb-platform/ydb-go-sdk/v3?status.svg)](https://godoc.org/github.com/ydb-platform/ydb-go-sdk/v3)
![tests](https://github.com/ydb-platform/ydb-go-sdk/workflows/tests/badge.svg?branch=master)
![lint](https://github.com/ydb-platform/ydb-go-sdk/workflows/lint/badge.svg?branch=master)
[![Go Report Card](https://goreportcard.com/badge/github.com/ydb-platform/ydb-go-sdk/v3)](https://goreportcard.com/report/github.com/ydb-platform/ydb-go-sdk/v3)
[![codecov](https://codecov.io/gh/ydb-platform/ydb-go-sdk/branch/master/graph/badge.svg?precision=2)](https://app.codecov.io/gh/ydb-platform/ydb-go-sdk)

> YDB API client written in Go.

[godoc](https://godoc.org/github.com/ydb-platform/ydb-go-sdk/v3/)

## Table of contents
1. [Overview](#Overview)
2. [About semantic versioning](#SemVer)
3. [About expected go1.18 changes](#go1.18)
4. [Prerequisites](#Prerequisites)
5. [Installation](#Install)
6. [Usage](#Usage)
7. [Credentials](#Credentials)
8. [Environment variables](#Environ)
9. [Ecosystem of debug tools](#Debug)
10. [Examples](#Examples)

## Overview <a name="Overview"></a>

Currently package ydb provides `scheme` and `table` API client implementations for `YDB`.

## About semantic versioning <a name="SemVer"></a>

We follow the **[SemVer 2.0.0](https://semver.org)**. In particular, we provide backward compatibility in the `MAJOR` releases. New features without loss of backward compatibility appear on the `MINOR` release. In the minor version, the patch number starts from `0`. Bug fixes and internal changes are released with the third digit (`PATCH`) in the version.

There are, however, some changes with the loss of backward compatibility that we consider to be `MINOR`:
* extension or modification of internal `ydb-go-sdk` interfaces. We understand that this will break the compatibility of custom implementations of the `ydb-go-sdk` internal interfaces. But we believe that the internal interfaces of `ydb-go-sdk` are implemented well enough that they do not require custom implementation. We are working to ensure that all internal interfaces have limited access only inside `ydb-go-sdk`.
* major changes to (including removal of) the public interfaces and types that have been previously exported by `ydb-go-sdk`. We understand that these changes will break the backward compatibility of early adopters of these interfaces. However, these changes are generally coordinated with early adopters and have the concise interfacing with `ydb-go-sdk` as a goal.

Internal interfaces outside from `internal` directory are marked with comment such as
```
// Warning: only for internal usage inside ydb-go-sdk
```

We publish the planned breaking `MAJOR` changes:
* via the comment `Deprecated` in the code indicating what should be used instead
* through the file [`NEXT_MAJOR_RELEASE.md`](#NEXT_MAJOR_RELEASE.md)

## About expected `go1.18` changes <a name="go1.18"></a>

Some changes from `go1.18` are expected and will be allowed to `ydb-go-sdk`:
- type set constraints instead abstract `interface{}`. Firstly, this changes will be applied to scanner API (`Scan`, `ScanWithDefaults` and `ScanNamed`)
- improve `log` package with parametrized types.

That changes could break backward compatibility.
We are tried to support `go1.18` features seamlessly. But now we was tested `go1.18-beta` implementation and detected some strong issues:
- go version in `go.mod` must be forced up to 1.18,
- interfaces with methods are not supported in type sets and other.

## Prerequisites <a name="Prerequisites"></a>

Requires `go1.14` or later.

## Installation <a name="Installation"></a>

```
go get -u github.com/ydb-platform/ydb-go-sdk/v3
```

## Usage <a name="Usage"></a>

The straightforward example of querying data may look similar to this:

```go
   ctx := context.Background()

   // ydb.New() returns connection object which provide necessary clients for different ydb services
   // such as table.Client, scheme.Client, coordination.Client, etc.
   db, err := ydb.New(
      ctx,
      ydb.WithConnectionString(os.Getenv("YDB_CONNECTION_STRING")),
      ydb.WithDialTimeout(3 * time.Second),
      ydb.WithCertificatesFromFile("~/.ydb/CA.pem"),
      ydb.WithSessionPoolIdleThreshold(time.Second * 5),
      ydb.WithSessionPoolKeepAliveMinSize(-1),
      ydb.WithDiscoveryInterval(5 * time.Second),
      ydb.WithAccessTokenCredentials(os.GetEnv("YDB_ACCESS_TOKEN_CREDENTIALS")),
   )
   if err != nil {
      // handle error
   }
   defer func() { _ = db.Close(ctx) }()

   // Prepare transaction control for upcoming query execution.
   // NOTE: result of TxControl() may be reused.
   txc := table.TxControl(
      table.BeginTx(table.WithSerializableReadWrite()),
      table.CommitTx(),
   )

   var res resultset.Result

   // Do() provide the best effort for executing operation
   // Do implements internal busy loop until one of the following conditions occurs:
   // - deadline was cancelled or deadlined
   // - operation returned nil as error
   // Note that in case of prepared statements call to Prepare() must be made
   // inside the function body.
   err := db.Table().Do(
      ctx, 
      func(ctx context.Context, s table.Session) (err error) {
         // Execute text query without preparation and with given "autocommit"
         // transaction control. That is, transaction will be committed without
         // additional calls. Notice the "_" unused variable – it stands for created
         // transaction during execution, but as said above, transaction is committed
         // for us and `ydb-go-sdk` do not want to do anything with it.
         _, res, err := s.Execute(ctx, txc,
            `--!syntax_v1
             DECLARE $mystr AS Utf8?;
             SELECT 42 as id, $mystr as mystr
             `,
            table.NewQueryParameters(
               table.ValueParam("$mystr", types.OptionalValue(types.UTF8Value("test"))),
            ),
         )
         return err
      },
   )
   if err != nil {
       return err // handle error
   }
   defer func() {
      _ = res.Close()
   }()
   // Scan for received values within the result set(s).
   // res.Err() reports the reason of last unsuccessful one.
   var (
       id    int32
       myStr *string //optional value
   )
   for res.NextResultSet(ctx, "id", "mystr") {
       for res.NextRow() {
           // Suppose our "users" table has two rows: id and age.
           // Thus, current row will contain two appropriate items with
           // exactly the same order.
           err := res.Scan(&id, &myStr)
   
           // Error handling.
           if err != nil {
               return err
           }
           // do something with data
           fmt.Printf("got id %v, got mystr: %v\n", id, *myStr)
       }
   }
   if res.Err() != nil {
       return res.Err() // handle error
   }
```

YDB sessions may become staled and appropriate error will be returned. To
reduce boilerplate overhead for such cases `ydb-go-sdk` provides generic retry logic

## Credentials <a name="Credentials"></a>

There are different variants to get `credentials.Credentials` object to get authorized.

Package | Type | Description                                                                                                                                                                                 | Link of example usage
--- | --- |---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------| ---
[ydb-go-yc](https://github.com/ydb-platform/ydb-go-yc) | credentials | credentials provider for Yandex.Cloud | [yc.WithServiceAccountKeyFileCredentials](https://github.com/ydb-platform/ydb-go-yc/blob/master/internal/cmd/connect/main.go#L22) [yc.WithInternalCA](https://github.com/ydb-platform/ydb-go-yc/blob/master/internal/cmd/connect/main.go#L22) [yc.WithMetadataCredentials](https://github.com/ydb-platform/ydb-go-yc/blob/master/internal/cmd/connect/main.go#L24) 
[ydb-go-yc-metadata](https://github.com/ydb-platform/ydb-go-yc-metadata) | credentials | metadata credentials provider for Yandex.Cloud | [yc.WithInternalCA](https://github.com/ydb-platform/ydb-go-yc-metadata/blob/master/options.go#L23) [yc.WithCredentials](https://github.com/ydb-platform/ydb-go-yc-metadata/blob/master/options.go#L17) 
[ydb-go-sdk-auth-environ](https://github.com/ydb-platform/ydb-go-sdk-auth-environ) | credentials | create credentials from environ | [ydbEnviron. WithEnvironCredentials](https://github.com/ydb-platform/ydb-go-sdk-auth-environ/blob/master/env.go#L11) 

Usage examples can be found [here](https://github.com/ydb-platform/ydb-go-examples/tree/master/cmd/auth).


## Environment variables <a name="Environ"></a>

Name | Type | Default | Description
--- | --- | --- | ---
`YDB_SSL_ROOT_CERTIFICATES_FILE` | `string` | | path to certificates file
`YDB_LOG_SEVERITY_LEVEL` | `string` | `quiet` | severity logging level. Supported: `trace`, `debug`, `info`, `warn`, `error`, `fatal`, `quiet`
`YDB_LOG_NO_COLOR` | `bool` | `false` | set any non empty value to disable colouring logs 
`GRPC_GO_LOG_VERBOSITY_LEVEL` | `integer` | | set to `99` to see grpc logs  
`GRPC_GO_LOG_SEVERITY_LEVEL` | `string` | | set to `info` to see grpc logs

## Ecosystem of debug tools over `ydb-go-sdk` <a name="Debug"></a>

Package ydb-go-sdk provide debugging over trace events in package `trace`. 
Now supports driver events in `trace.Driver` struct and table-service events in `trace.Table` struct.
Next packages provide debug tooling:

Package | Type | Description                                                                                                                                                                                 | Link of example usage
--- | --- |---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------| ---
[ydb-go-sdk-zap](https://github.com/ydb-platform/ydb-go-sdk-zap) | logging | logging ydb-go-sdk events with zap package                                                                                                                                                  | [ydbZap.WithTraces](https://github.com/ydb-platform/ydb-go-sdk-zap/blob/master/internal/cmd/bench/main.go#L64)
[ydb-go-sdk-zerolog](https://github.com/ydb-platform/ydb-go-sdk-zap) | logging | logging ydb-go-sdk events with zerolog package                                                                                                                                              | [ydbZerolog.WithTraces](https://github.com/ydb-platform/ydb-go-sdk-zerolog/blob/master/internal/cmd/bench/main.go#L47)
[ydb-go-sdk-metrics](https://github.com/ydb-platform/ydb-go-sdk-metrics) | metrics | common metrics of ydb-go-sdk. Package declare interfaces such as `Registry`, `GaugeVec` and `Gauge` and use it for create `trace.Driver` and `trace.Table` traces                           |
[ydb-go-sdk-prometheus](https://github.com/ydb-platform/ydb-go-sdk-prometheus) | metrics | prometheus wrapper over [ydb-go-sdk-metrics](https://github.com/ydb-platform/ydb-go-sdk-metrics) | [ydbPrometheus.WithTraces](https://github.com/ydb-platform/ydb-go-sdk-prometheus/blob/master/internal/cmd/bench/main.go#L56)
[ydb-go-sdk-opentracing](https://github.com/ydb-platform/ydb-go-sdk-opentracing) | tracing | opentracing plugin for trace internal ydb-go-sdk calls | [ydbOpentracing.WithTraces](https://github.com/ydb-platform/ydb-go-sdk-opentracing/blob/master/internal/cmd/bench/main.go#L86) 

## Examples <a name="Examples"></a>

More examples are listed in [examples](https://github.com/ydb-platform/ydb-go-examples) repository.

