"use strict"

const searchFilters = document.getElementById('search-filters-offcanvas')
const searchbox = document.getElementById('searchbox');
searchFilters.addEventListener('hide.bs.offcanvas', () => {
    searchFilters.querySelectorAll('input[type="search"]').forEach(input => {
        searchbox.value = input.value;
    });
})
searchFilters.addEventListener('show.bs.offcanvas', () => {
    searchFilters.querySelectorAll('input[type="search"]').forEach(input => {
        input.value = searchbox.value;
    });
})
