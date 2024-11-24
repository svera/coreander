"use strict"

// We use several conventions to be able to use the same code to delete different resources.
// The link that initiates the action needs to have an attribute called data-id which must contain an unique identifier
// for the resource to delete.
// This identifier well be sent to the backend controller specified in the form's action attribute
// under the name "id".
// This code is designed to be used alongside partials/delete-modal.html

const deleteModal = document.getElementById('delete-modal');
const deleteForm = document.getElementById('delete-form');
let id

deleteModal.addEventListener('show.bs.modal', event => {
    const link = event.relatedTarget
    id = link.getAttribute('data-id')
})

deleteModal.addEventListener('hidden.bs.modal', event => {
    let message = document.getElementById('error-message-container');
    message.classList.add("visually-hidden");
})

deleteForm.addEventListener('submit', event => {
    event.preventDefault();
    console.log(deleteForm.getAttribute("action") + '/' + id, 'ruta')
    fetch(deleteForm.getAttribute("action") + '/' + id, {
        method: "DELETE"
    })
    .then((response) => {
        if (response.ok || response.status == "403") {
            location.reload();
        } else {
            let message = document.getElementById("error-message-container");
            message.classList.remove("visually-hidden");
            message.innerHTML = deleteForm.getAttribute("data-error-message");
        }
    })
    .catch(function (error) {
        // Catch errors
        console.log(error);
    });
})
