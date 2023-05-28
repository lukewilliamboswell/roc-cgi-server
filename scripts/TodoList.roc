app "todo-list"
    packages {
        pf: "https://github.com/agu-z/roc-basic-cli/releases/download/0.5.0/S8r4wytSGYKi-iMrjaRZxv2Hope_CX7dF6rMdciYja8.tar.gz",
        pg: "https://github.com/lukewilliamboswell/roc-package-explorations/releases/download/pg/t0RDJ33m-ZCs7QLL2OmttKNqhB026CRZl9XcAgQAdFc.tar.br",
    }
    imports [
        pf.Task.{ Task, await },
        pf.Process,
        pf.Stdout,
        pf.Stderr,
        pg.Pg.Cmd,
        pg.Pg.Client,
        pg.Pg.Result,
        Html.{ html, head, body, div, text, a, h1, link, p, h5, hr, meta },
        Html.Attributes.{ httpEquiv, content, href, crossorigin, style, integrity, rel, lang, class },
    ]
    provides [main] to pf

task =
    # Connect to the database
    client <- Pg.Client.withConnect {
            host: "localhost",
            port: 5432,
            user: "test",
            auth: Password "test",
            database: "todo",
        }

    # Get the list of todos from the database
    todos <-
        Pg.Cmd.new
            """
            select id, task
            from todo
            """
        |> Pg.Cmd.expectN
            (
                Pg.Result.succeed
                    (\id -> \title -> {id, title})
                |> Pg.Result.with (Pg.Result.str "id")
                |> Pg.Result.with (Pg.Result.str "task")
            )
        |> Pg.Client.command client
        |> await

    # Process and return the HTML
    todos 
    |> viewTodos
    |> Html.render
    |> Stdout.line

main : Task {} []
main =
    Task.attempt task \result ->
        when result is
            Ok _ ->
                Process.exit 0

            Err (TcpPerformErr (PgErr err)) ->
                _ <- Stderr.line (Pg.Client.errorToStr err) |> await
                Process.exit 2

            Err err ->
                dbg
                    err

                _ <- Stderr.line "Something went wrong" |> await
                Process.exit 1


viewTodos : List {id : Str, title : Str} -> Html.Node
viewTodos = \todos ->
    html [lang "en"] [
        head [] [
            meta [httpEquiv "content-type", content "text/html; charset=utf-8"] [],
            Html.title [] [text "Todo List"],
            link [
                rel "stylesheet", 
                href "https://cdn.jsdelivr.net/npm/bootstrap@5.3.0-alpha3/dist/css/bootstrap.min.css",
                integrity "sha384-KK94CHFLLe+nY2dmCWGMq91rCGa5gtU4mk92HdvYe+M/SXH301p5ILy+dN9+nJOZ",
                crossorigin "anonymous",
            ] [],
        ],
        body [] [
            div [class "container", style "margin-top: 50px;"] [
                h1 [] [text "Lits of Todos"],
                hr [] [],
                div [class "row"] [
                    div [class "col-md-8"] (
                        List.map todos \todo ->
                            div [class "card mt-3"] [
                                div [class "card-body"] [
                                    h5 [class "card-title"] [text todo.id],
                                    p [class "card-text"] [text todo.title],
                                    a [href (Str.concat "/todo/" todo.id), class "btn btn-primary"] [text "Todo Detail"],
                                ]
                            ]
                    )
                ],

                div [class "row mt-4"] [
                    div [class "col-md-6"] [
                        a [href "#", class "btn btn-primary"] [text "Add Todo"],
                        a [href "#", class "btn btn-danger"] [text "Delete All"],
                    ]
                ]
            ]
        ],
    ]

    

    