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

function send(index) {
    event.preventDefault();
    let formID = "send-email-" + index;
    let submit = document.querySelector('#'+formID+' button');
    let envelope = document.querySelector('#envelope-'+index);
    let spinner = document.querySelector('#spinner-'+index);
    form = document.getElementById(formID);
    submit.setAttribute("disabled", true);
    spinner.classList.remove("visually-hidden");
    envelope.classList.add("visually-hidden");
    fetch('/send', {
        method: "POST",
        headers: {
            'Content-Type': 'application/x-www-form-urlencoded'
        },
        body: new URLSearchParams({
            'email': form.elements[0].value,
            'slug': form.elements[1].value,        
        })
    })
    .then((response) => {
        message = document.getElementById("send-email-message-" + index)
        message.classList.remove("visually-hidden");
        if (!response.ok) {
            message.innerHTML = '{{t .Lang "There was an error sending the document, please try again later"}}';
            message.classList.add("text-danger");
        } else {
            message.innerHTML = '{{t .Lang "Document sent succesfully"}}';
            message.classList.add("text-success");
        }
        submit.removeAttribute("disabled");
        envelope.classList.remove("visually-hidden");
        spinner.classList.add("visually-hidden");
    })
    .catch(function (error) {
        // Catch errors
        console.log(error);
    });
}

function remove() {
    event.preventDefault();
    form = document.getElementById("delete-form");
    fetch('/delete', {
        method: "POST",
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
            message.innerHTML = '{{t .Lang "There was an error deleting the document"}}';
        }
    })
    .catch(function (error) {
        // Catch errors
        console.log(error);
    });
}

function highlight(index) {
    event.preventDefault();
    let highlightFormParent = document.querySelector("#highlight-" + index);
    let dehighlightFormParent = document.querySelector("#dehighlight-" + index);
    let highlightForm = highlightFormParent.querySelector("form");
    let submit = highlightForm.querySelector('button');
    fetch('/highlight', {
        method: "POST",
        headers: {
            'Content-Type': 'application/x-www-form-urlencoded'
        },
        body: new URLSearchParams({
            'slug': highlightForm.elements['slug'].value,        
        })
    })
    .then((response) => {
        if (response.ok) {
            highlightFormParent.classList.add("visually-hidden");
            dehighlightFormParent.classList.remove("visually-hidden");
        } else {
            console.log(response.body)
        }
    })
    .catch(function (error) {
        // Catch errors
        console.log(error);
    });
}

function dehighlight(index, slug, el) {
    event.preventDefault();
    let highlightFormParent = document.querySelector("#highlight-" + index);
    let dehighlightLinkParent = document.querySelector("#dehighlight-" + index);
    fetch(
        el.getAttribute("href"), {
            method: "DELETE",
            headers: {
                'Content-Type': 'application/x-www-form-urlencoded'
            },
            credentials: "same-origin",
            body: new URLSearchParams({
                'slug': slug,        
            })
        }
    )
    .then((response) => {
        if (response.ok) {
            dehighlightLinkParent.classList.add("visually-hidden");
            highlightFormParent.classList.remove("visually-hidden");
        } else {
            console.log(response.body)
        }
    })
    .catch(function (error) {
        // Catch errors
        console.log(error);
    });
}

window.onload = function() {
    let imgs = document.querySelectorAll('.cover');
    for (i = 0; i < imgs.length; i++) {
        if (imgs[i].getAttribute('data-src')) {
            imgs[i].addEventListener('error', function onError(e) {
                this.setAttribute('src', '/images/generic.png');
            });
            imgs[i].setAttribute('src', imgs[i].getAttribute('data-src'));
        }
    }
}