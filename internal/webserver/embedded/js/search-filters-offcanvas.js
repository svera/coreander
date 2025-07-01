const searchFilters = document.getElementById('search-filters-offcanvas')
const searchbox = document.getElementById('searchbox');
searchFilters.addEventListener('hide.bs.offcanvas', event => {
    searchFilters.querySelectorAll('input[type="search"]').forEach(input => {
        searchbox.value = input.value;
    });
})
searchFilters.addEventListener('show.bs.offcanvas', event => {
    searchFilters.querySelectorAll('input[type="search"]').forEach(input => {
        input.value = searchbox.value;
    });
})
