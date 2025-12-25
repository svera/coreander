// Use event delegation to handle clicks on author images loaded via HTMX
document.addEventListener('click', function(e) {
  const img = e.target.closest('.author-image-upload');
  if (!img?.dataset.authorSlug || img.dataset.uploading === 'true') {
    return;
  }

  e.preventDefault();
  e.stopPropagation();

  const fileInput = document.createElement('input');
  fileInput.type = 'file';
  fileInput.accept = 'image/jpeg,image/jpg,image/png';
  fileInput.style.display = 'none';

  fileInput.addEventListener('change', function(e) {
    const file = e.target.files[0];
    if (!file) return;

    const allowedTypes = ['image/jpeg', 'image/jpg', 'image/png'];
    if (!allowedTypes.includes(file.type)) {
      showToast(img.dataset.invalidFileType || 'Invalid file type. Only JPEG and PNG images are allowed.', 'danger');
      return;
    }

    const authorSlug = img.dataset.authorSlug;
    img.dataset.uploading = 'true';
    img.style.opacity = '0.5';
    img.style.cursor = 'wait';

    const formData = new FormData();
    formData.append('image', file);

    fetch(`/authors/${authorSlug}/image`, {
      method: 'POST',
      body: formData
    })
    .then(response => response.json())
    .then(data => {
      img.dataset.uploading = 'false';
      img.style.opacity = '1';
      img.style.cursor = '';

      if (data.success) {
        reloadAuthorImage(img);
      } else {
        showToast(data.error || img.dataset.uploadFailed || 'Failed to upload image', 'danger');
      }
    })
    .catch(error => {
      console.error('Error:', error);
      img.dataset.uploading = 'false';
      img.style.opacity = '1';
      img.style.cursor = '';
      showToast(img.dataset.uploadError || 'An error occurred while uploading the image', 'danger');
    });
  });

  document.body.appendChild(fileInput);
  fileInput.click();
  setTimeout(() => fileInput.remove(), 100);
});

function reloadAuthorImage(img) {
  const htmxContainer = img.closest('[hx-get]');
  if (!htmxContainer?.getAttribute('hx-get')) {
    // Fallback: update image src directly
    const currentSrc = img.src.split('?')[0];
    img.src = '';
    setTimeout(() => img.src = `${currentSrc}?t=${Date.now()}`, 10);
    return;
  }

  const handleReload = function(event) {
    if (event.detail.target === htmxContainer) {
      const newImg = htmxContainer.querySelector('img[src*="/authors/"]');
      if (newImg) {
        const imgSrc = newImg.src.split('?')[0];
        newImg.src = `${imgSrc}?t=${Date.now()}`;
      }
      document.body.removeEventListener('htmx:afterSwap', handleReload);
    }
  };

  document.body.addEventListener('htmx:afterSwap', handleReload);

  const url = htmxContainer.getAttribute('hx-get');
  const separator = url.includes('?') ? '&' : '?';
  htmx.ajax('GET', `${url}${separator}_t=${Date.now()}`, {
    target: htmxContainer,
    swap: 'outerHTML'
  });
}

function showToast(message, type) {
  const toastId = type === 'danger' ? 'live-toast-danger' : 'live-toast-success';
  const toast = document.getElementById(toastId);
  if (!toast) return;

  toast.querySelector('.toast-body').innerHTML = message;
  const toastBootstrap = bootstrap.Toast.getOrCreateInstance(toast);
  toastBootstrap.show();
}

