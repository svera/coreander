"use strict";

// Control checkbox state when marking a document as complete/incomplete
document.body.addEventListener('htmx:afterRequest', function(evt) {
    if (!evt.detail.elt.id || !evt.detail.elt.id.startsWith("complete-checkbox-")) {
        return
    }
    if (!evt.detail.successful) {
        // If request failed, revert the checkbox state
        const checkboxEl = evt.detail.elt;
        checkboxEl.checked = !checkboxEl.checked;
        return
    }

    const checkboxEl = evt.detail.elt;

    // Update the title based on the new state
    if (checkboxEl.checked) {
        checkboxEl.title = checkboxEl.getAttribute('data-incomplete-title');
    } else {
        checkboxEl.title = checkboxEl.getAttribute('data-complete-title');
    }
});

// Handle completion date changes
document.addEventListener('DOMContentLoaded', function() {
    // Add hover and focus effects for date inputs
    document.body.addEventListener('mouseover', function(evt) {
        if (evt.target.id && evt.target.id.startsWith("completion-date-")) {
            evt.target.style.textDecoration = 'underline';
        }
    });

    document.body.addEventListener('mouseout', function(evt) {
        if (evt.target.id && evt.target.id.startsWith("completion-date-")) {
            evt.target.style.textDecoration = 'none';
        }
    });

    document.body.addEventListener('focus', function(evt) {
        if (evt.target.id && evt.target.id.startsWith("completion-date-")) {
            evt.target.classList.add('border', 'border-primary', 'px-2', 'rounded');
        }
    }, true);

    document.body.addEventListener('blur', function(evt) {
        if (evt.target.id && evt.target.id.startsWith("completion-date-")) {
            evt.target.classList.remove('border', 'border-primary', 'px-2', 'rounded');
        }
    }, true);

    document.body.addEventListener('change', function(evt) {
        if (!evt.target.id || !evt.target.id.startsWith("completion-date-")) {
            return;
        }

        const input = evt.target;
        const slug = input.dataset.slug;
        const newDate = input.value;
        const originalDate = input.dataset.originalDate;

        // Only update if the date actually changed
        if (newDate === originalDate) {
            return;
        }

        // Validate date format
        if (!newDate || !/^\d{4}-\d{2}-\d{2}$/.test(newDate)) {
            alert('Invalid date format');
            input.value = originalDate;
            return;
        }

        // Send the update request
        fetch(`/documents/${slug}/complete`, {
            method: 'PUT',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({ completed_at: newDate })
        })
        .then(response => {
            if (response.ok) {
                // Update the original date to the new value
                input.dataset.originalDate = newDate;
                // Show subtle feedback with green border
                input.classList.add('border', 'border-success', 'px-2', 'rounded');
                setTimeout(() => {
                    input.classList.remove('border', 'border-success', 'px-2', 'rounded');
                }, 1000);
            } else {
                throw new Error('Failed to update date');
            }
        })
        .catch(error => {
            console.error('Error updating completion date:', error);
            alert('Failed to update completion date. Please try again.');
            // Revert to original date
            input.value = originalDate;
        });
    });
});


