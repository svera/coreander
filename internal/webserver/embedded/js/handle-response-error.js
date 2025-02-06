export const handleResponseError = (evt) => {
    if (evt.detail.xhr.status === 403) {
        location.reload()
        return
    }

    const toast = document.getElementById('live-toast-danger')
    toast.querySelector(".toast-body").innerHTML = evt.detail.elt.getAttribute("data-error-message")
    const toastBootstrap = bootstrap.Toast.getOrCreateInstance(toast)
    toastBootstrap.show()
}
