document.body.addEventListener('htmx:afterRequest', function (evt) {
    const errorTarget = document.getElementById("box-error")
    if (evt.detail.successful) {
        // Successful request, clear out alert
        errorTarget.setAttribute("hidden", "true")
        errorTarget.innerText = "";
    } else if (evt.detail.failed && evt.detail.xhr) {
        // Server error with response contents, equivalent to htmx:responseError
        const xhr = evt.detail.xhr;
        if (xhr.status == "403") {
            location.reload()
        }
        if (xhr.status == "400") {
            return
        }

        console.warn("Server error", evt.detail)
        errorTarget.innerText = `Unexpected server error: ${xhr.status} - ${xhr.statusText}`;
        errorTarget.removeAttribute("hidden");
    } else {
        // Unspecified failure, usually caused by network error
        console.error("Unexpected htmx error", evt.detail)
        errorTarget.innerText = "Unexpected error, check your connection and try to refresh the page.";
        errorTarget.removeAttribute("hidden");
    }
});