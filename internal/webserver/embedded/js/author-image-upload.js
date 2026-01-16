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
    // Use provided timestamp from server (file modification time)
    // This ensures the URL matches what the server will generate
    const cacheBuster = timestamp || Date.now();

    // Extract base URL (remove any existing query parameters)
    // Handle both absolute and relative URLs
    let baseUrl;
    try {
      const urlObj = new URL(img.src, window.location.origin);
      baseUrl = urlObj.pathname;
    } catch (e) {
      // If URL parsing fails (relative URL), extract pathname manually
      baseUrl = img.src.split('?')[0];
    }

    const newSrc = baseUrl + '?t=' + cacheBuster;

    // Use fetch with cache: 'no-store' to force a fresh download
    // This bypasses browser cache completely
    fetch(newSrc, {
      cache: 'no-store',
      headers: {
        'Cache-Control': 'no-cache',
        'Pragma': 'no-cache'
      }
    })
    .then(response => {
      if (!response.ok) {
        throw new Error('Failed to fetch image');
      }
      return response.blob();
    })
    .then(blob => {
      // Create object URL from blob to force browser to display new image
      const objectUrl = URL.createObjectURL(blob);

      // Clean up old object URL if it exists
      if (img.dataset.objectUrl) {
        URL.revokeObjectURL(img.dataset.objectUrl);
      }

      // Store object URL for cleanup later
      img.dataset.objectUrl = objectUrl;

      // Update image src with object URL
      img.src = objectUrl;

      // After image loads, optionally switch to the regular URL
      // This allows the browser to cache it normally going forward
      img.onload = function() {
        // Small delay to ensure image is displayed
        setTimeout(() => {
          if (img.dataset.objectUrl) {
            URL.revokeObjectURL(img.dataset.objectUrl);
            delete img.dataset.objectUrl;
            // Switch to regular URL with cache buster
            img.src = newSrc;
          }
        }, 100);
      };
    })
    .catch(error => {
      console.error('Error fetching image:', error);
      // Fallback: try direct assignment
      img.src = newSrc;
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
