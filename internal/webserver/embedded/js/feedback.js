"use strict"

htmx.on("htmx:beforeSwap", (evt) => {
    // Allow 422 and 400 responses to swap
    // We treat these as form validation errors
    if (evt.detail.xhr.status === 422 || evt.detail.xhr.status === 400) {
        evt.detail.shouldSwap = true;
        evt.detail.isError = false;
    }
})

let dataSuccessMessage, dataErrorMessage = null

htmx.on('htmx:beforeRequest', (evt) => {
    dataSuccessMessage = evt.detail.elt.getAttribute("data-success-message")
    dataErrorMessage = evt.detail.elt.getAttribute("data-error-message")
})

htmx.on('htmx:afterRequest', (evt) => {
    const toastSuccess = document.getElementById('live-toast-success')
    const toastDanger = document.getElementById('live-toast-danger')
    const unexpectedServerErrorText = document.getElementsByTagName('main')[0].dataset.unexpectedServerError

    if (!evt.detail.xhr) {
        return;
    }
    const xhr = evt.detail.xhr;

    if (evt.detail.failed) {
        // Server error with response contents, equivalent to htmx:responseError
        if (xhr.status === 403) {
            return location.reload()
        }

        if (xhr.status >= 500) {
            console.warn("Server error", evt.detail)
            toastDanger.querySelector(".toast-body").innerHTML = unexpectedServerErrorText + `${xhr.status} - ${xhr.statusText}`
            const toastBootstrap = bootstrap.Toast.getOrCreateInstance(toastDanger)
            toastBootstrap.show()
            return
        }

        if (dataErrorMessage !== null) {
            toastDanger.querySelector(".toast-body").innerHTML = dataErrorMessage
            const toastBootstrap = bootstrap.Toast.getOrCreateInstance(toastDanger)
            toastBootstrap.show()
            return
        }
    }

    if (xhr.status === 200 && dataSuccessMessage !== null && dataSuccessMessage !== undefined) {
        toastSuccess.querySelector(".toast-body").innerHTML = dataSuccessMessage
        const toastBootstrap = bootstrap.Toast.getOrCreateInstance(toastSuccess)
        toastBootstrap.show()
    }
});

const setUpWarningsListener = () => {
    document.querySelectorAll('[data-warning-once]').forEach((el) => {
        el.addEventListener('click', () => {
            document.cookie = `warning-once=${el.getAttribute('data-warning-once')}; path=/`;
        });
    });
}

const warningsObserver = new MutationObserver(setUpWarningsListener);

// Start observing the target node for configured mutations
const node = document.getElementsByTagName("body")[0];
warningsObserver.observe(node, { attributes: true, childList: false, subtree: true });
