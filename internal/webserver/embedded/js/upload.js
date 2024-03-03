  const fileSelector = document.getElementById('file-selector');
  const uploadForm = document.getElementById('upload-form');
  let fileSubmit = document.getElementById('file-submit');

  fileSelector.addEventListener('change', (event) => {
    const fileList = Array.from(event.target.files);
    let fileSelector = document.getElementById('file-selector');
    let errorMessageContainer = document.getElementsByClassName('invalid-feedback')[0];

    fileList.forEach(element => {
        if (element.size > fileSelector.dataset.max_size * 1024 * 1024) {
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

  uploadForm.addEventListener('submit', (event) => {
    let spinner = document.querySelector('.spinner-border');
    spinner.classList.remove('visually-hidden')
    fileSubmit.setAttribute('disabled', '');
  });