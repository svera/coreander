const deleteModal = document.getElementById('deleteModal');

deleteModal.addEventListener('show.bs.modal', event => {
    const link = event.relatedTarget;
    const slug = link.getAttribute('data-bs-slug');
    const modalInputSlug = deleteModal.querySelector('.slug');

    modalInputSlug.value = slug;
})

deleteModal.addEventListener('hidden.bs.modal', event => {
    let message = document.getElementById('delete-document-message');
    message.classList.add("visually-hidden");
})

function remove(errorMessage) {
    event.preventDefault();
    form = document.getElementById("delete-form");
    fetch('/document', {
        method: "DELETE",
        headers: {
            'Content-Type': 'application/x-www-form-urlencoded'
        },
        body: new URLSearchParams({
            'slug': form.elements['slug'].value,        
        })
    })
    .then((response) => {
        if (response.ok) {
            location.reload();
        } else {
            message = document.getElementById("delete-document-message")
            message.classList.remove("visually-hidden");
            message.innerHTML = errorMessage;
        }
    })
    .catch(function (error) {
        // Catch errors
        console.log(error);
    });
}
