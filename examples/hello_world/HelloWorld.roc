app "hello-world"
    packages { pf: "https://github.com/roc-lang/basic-cli/releases/download/0.5.0/Cufzl36_SnJ4QbOoEmiJ5dIpUxBvdB3NEySvuH82Wio.tar.br" }
    imports [
        pf.Stdout,
        pf.Stderr,
        pf.Env,
        pf.Task.{ Task},
    ]
    provides [main] to pf

Request : {
    method : Str,
    uri : Str,
    queryStr : Str,
    contentLength : Str,
    contentType : Str,
    remoteAddr : Str,
    serverProtocol : Str,
    serverSoftware : Str,
    scriptName : Str,
}

main = 
    result <- Task.attempt parseRequestMeta
    
    when result is
        Ok req ->
            Stdout.line (printRequest req)
        Err _ ->
            Stderr.line "FAILURE"

parseRequestMeta : Task Request [VarNotFound]
parseRequestMeta =
    method <- Env.var "REQUEST_METHOD" |> Task.await
    uri <- Env.var "REQUEST_URI" |> Task.await
    queryStr <- Env.var "QUERY_STRING" |> Task.await
    contentLength <- Env.var "CONTENT_LENGTH" |> Task.await
    contentType <- Env.var "CONTENT_TYPE" |> Task.await
    remoteAddr <- Env.var "REMOTE_ADDR" |> Task.await
    serverProtocol <- Env.var "SERVER_PROTOCOL" |> Task.await
    serverSoftware <- Env.var "SERVER_SOFTWARE" |> Task.await
    scriptName <- Env.var "SCRIPT_NAME" |> Task.await

    Task.ok {
        method,
        uri,
        queryStr,
        contentLength,
        contentType,
        remoteAddr,
        serverProtocol,
        serverSoftware,
        scriptName,
    }

printRequest : Request -> Str 
printRequest = \req -> 
    [
        "REQUEST_METHOD=", req.method,
        "\nREQUEST_URI=", req.uri,
        "\nQUERY_STRING=", req.queryStr,
        "\nCONTENT_LENGTH=", req.contentLength,
        "\nCONTENT_TYPE=", req.contentType,
        "\nREMOTE_ADDR=", req.remoteAddr,
        "\nSERVER_PROTOCOL=", req.serverProtocol,
        "\nSERVER_SOFTWARE=", req.serverSoftware,
        "\nSCRIPT_NAME=", req.scriptName,
        "\n",
    ]
    |> Str.joinWith ""
