"use strict"

// We use several conventions to be able to use the same code to delete different resources.
// The link that initiates the action needs to have an attribute called data-id which must contain an unique identifier
// for the resource to delete.
// This identifier well be sent to the backend controller specified in the form's action attribute
// under the name "id".
// This code is designed to be used alongside partials/delete-modal.html

const deleteModal = document.getElementById('delete-modal');
const deleteForm = document.getElementById('delete-form');

deleteModal.addEventListener('show.bs.modal', event => {
    const link = event.relatedTarget
    const id = link.getAttribute('data-id')
    const modalInput = deleteModal.querySelector('.id')

    modalInput.value = id;
})

deleteModal.addEventListener('hidden.bs.modal', event => {
    let message = document.getElementById('error-message-container');
    message.classList.add("visually-hidden");
})

deleteForm.addEventListener('submit', event => {
    event.preventDefault();
    fetch(deleteForm.getAttribute("action"), {
        method: "DELETE",
        headers: {
            'Content-Type': 'application/x-www-form-urlencoded'
        },
        body: new URLSearchParams({
            'id': deleteForm.elements['id'].value,        
        })
    })
    .then((response) => {
        if (response.ok) {
            location.reload();
        } else {
            message = document.getElementById("error-message-container")
            message.classList.remove("visually-hidden");
            message.innerHTML = deleteForm.getAttribute("data-error-message");
        }
    })
    .catch(function (error) {
        // Catch errors
        console.log(error);
    });
})
