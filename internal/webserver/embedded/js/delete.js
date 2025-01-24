"use strict"

// We use several conventions to be able to use the same code to delete different resources.
// The link that initiates the action needs to have an attribute called data-id which must contain an unique identifier
// for the resource to delete.
// This identifier well be sent to the backend controller specified in the form's action attribute
// under the name "id".
// This code is designed to be used alongside partials/delete-modal.html

const deleteModal = document.getElementById('delete-modal');
const deleteForm = document.getElementById('delete-form');
let url

deleteModal.addEventListener('show.bs.modal', event => {
    const link = event.relatedTarget
    deleteForm.setAttribute('hx-delete', link.getAttribute('data-url'))
    htmx.process(deleteForm)
})

document.body.addEventListener('htmx:responseError', function (evt) {
    if (evt.detail.xhr.status === 403) {
        location.reload()
        return
    }

    const toast = document.getElementById('live-toast-danger')
    toast.querySelector(".toast-body").innerHTML = deleteForm.getAttribute("data-error-message")
    const toastBootstrap = bootstrap.Toast.getOrCreateInstance(toast)
    toastBootstrap.show()
})

document.body.addEventListener('htmx:afterRequest', function (evt) {
    const del = evt.detail.elt.getAttribute("hx-delete")
    if (!evt.detail.failed && del) {
        htmx.trigger("#list", "update")
    }
})
