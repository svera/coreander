document.body.addEventListener('htmx:afterRequest', function (evt) {
    const errorTarget = document.getElementById("box-error")
    const unexpectedServerError = errorTarget.getAttribute("data-unexpected-server-error")
    const unexpectedError = errorTarget.getAttribute("data-unexpected-error")
    if (evt.detail.successful) {
        // Successful request, clear out alert
        errorTarget.setAttribute("hidden", "true")
        errorTarget.innerText = "";
    } else if (evt.detail.failed && evt.detail.xhr) {
        // Server error with response contents, equivalent to htmx:responseError
        const xhr = evt.detail.xhr;
        if (xhr.status == "403") {
            return location.reload()
        }

        console.warn("Server error", evt.detail)
        errorTarget.innerText = unexpectedServerError + `${xhr.status} - ${xhr.statusText}`
        errorTarget.removeAttribute("hidden")
    } else {
        // Unspecified failure, usually caused by network error
        console.error("Unexpected htmx error", evt.detail)
        errorTarget.innerText = unexpectedError
        errorTarget.removeAttribute("hidden")
    }
});
