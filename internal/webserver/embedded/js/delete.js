"user strict"

const deleteModal = document.getElementById('deleteModal');
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
