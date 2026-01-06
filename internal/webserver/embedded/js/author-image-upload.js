// Prevent multiple initializations if script is loaded multiple times
(function() {
  if (window.authorImageUploadInitialized) {
    return; // Script already initialized, skip
  }
  window.authorImageUploadInitialized = true;

  // Create a single reusable file input instance
  const fileInput = document.createElement('input');
  fileInput.type = 'file';
  fileInput.accept = 'image/jpeg,image/jpg,image/png';
  fileInput.style.display = 'none';
  document.body.appendChild(fileInput);

  // Store reference to the currently clicked image
  let currentImg = null;

  fileInput.addEventListener('change', function(e) {
    const file = e.target.files[0];
    if (!file || !currentImg) {
      currentImg = null;
      return;
    }

    const img = currentImg;
    currentImg = null;

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
    .then(response => {
      img.dataset.uploading = 'false';
      img.style.opacity = '1';
      img.style.cursor = '';

      if (response.ok) {
        // Get timestamp from response header for cache busting
        const timestamp = response.headers.get('X-Image-Timestamp');
        reloadAuthorImage(img, timestamp);
        return;
      }

      // Error response - try to parse JSON error message
      return response.json()
        .then(data => {
          showToast(data.error || img.dataset.uploadFailed || 'Failed to upload image', 'danger');
        })
        .catch(() => {
          // If JSON parsing fails, show generic error
          showToast(img.dataset.uploadFailed || 'Failed to upload image', 'danger');
        });
    })
    .catch(error => {
      console.error('Error:', error);
      img.dataset.uploading = 'false';
      img.style.opacity = '1';
      img.style.cursor = '';
      showToast(img.dataset.uploadError || 'An error occurred while uploading the image', 'danger');
    });

    // Reset file input so the same file can be selected again if needed
    fileInput.value = '';
  });

  // Use event delegation to handle clicks on author images loaded via HTMX
  document.addEventListener('click', function(e) {
    const img = e.target.closest('.author-image-upload');
    if (!img?.dataset.authorSlug || img.dataset.uploading === 'true') {
      return;
    }

    e.preventDefault();
    e.stopPropagation();

    // Store reference to clicked image
    currentImg = img;

    // Trigger file input click
    fileInput.click();
  });

  function reloadAuthorImage(img, timestamp) {
    // Use provided timestamp or generate a new one
    const cacheBuster = timestamp || Date.now();

    const htmxContainer = img.closest('[hx-get]');
    if (!htmxContainer?.getAttribute('hx-get')) {
      // Fallback: update image src directly with cache buster
      const currentSrc = img.src.split('?')[0];
      img.src = '';
      setTimeout(() => img.src = `${currentSrc}?t=${cacheBuster}`, 10);
      return;
    }

    const handleReload = function(event) {
      if (event.detail.target === htmxContainer) {
        const newImg = htmxContainer.querySelector('img[src*="/authors/"]');
        if (newImg) {
          // Force reload with cache buster to bypass browser cache
          const imgSrc = newImg.src.split('?')[0];
          newImg.src = '';
          setTimeout(() => {
            newImg.src = `${imgSrc}?t=${cacheBuster}`;
          }, 10);
        }
        document.body.removeEventListener('htmx:afterSwap', handleReload);
      }
    };

    document.body.addEventListener('htmx:afterSwap', handleReload);

    const url = htmxContainer.getAttribute('hx-get');
    const separator = url.includes('?') ? '&' : '?';
    htmx.ajax('GET', `${url}${separator}_t=${cacheBuster}`, {
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
})();
