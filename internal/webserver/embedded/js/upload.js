  const fileSelector = document.getElementById('file-selector');
  fileSelector.addEventListener('change', (event) => {
    const fileList = Array.from(event.target.files);
    let fileSubmit = document.getElementById('file-submit');
    let fileSelector = document.getElementById('file-selector');
    let errorMessageContainer = document.getElementsByClassName('invalid-feedback')[0];
    
    fileList.forEach(element => {
        if (element.size > fileSelector.dataset.max_size) {
            fileSubmit.setAttribute('disabled', '');
            fileSelector.classList.add('is-invalid');
            errorMessageContainer.classList.remove('visually-hidden');
            errorMessageContainer.textContent = fileSelector.dataset.error_too_large;
        } else {
            fileSubmit.removeAttribute('disabled');
            fileSelector.classList.remove('is-invalid');
            errorMessageContainer.classList.add('visually-hidden');
            errorMessageContainer.textContent = '';
        }
    });
  });
