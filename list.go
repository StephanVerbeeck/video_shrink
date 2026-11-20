package main

// De generieke List container (e.g.  type Books List[Book])
type List[T comparable] []T

// make the list empty (delete all items from it)
func (list *List[T]) Empty() {
	*list = nil
}

// returns true when the list is empty
func (list List[T]) IsEmpty() bool {
	return len(list) == 0
}

// returns true if the list contains the given item
func (list List[T]) Contains(value T) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}

// Add item to end of list
func (list *List[T]) Add(value T) {
	*list = append(*list, value)
}

// AddUnique add extra string to start of list of unique strings
func (list *List[T]) AddUnique(value T) {
	if !list.Contains(value) {
		list.Add(value)
	}
}

// Prepend voegt een item toe aan het begin van de lijst
func (l *List[T]) Prepend(value T) {
	if l == nil {
		return
	}
	*l = append([]T{value}, *l...)
}

// PrependUnique add extra string to start of list of unique strings
func (list *List[T]) PrependUnique(value T) {
	if !list.Contains(value) {
		list.Prepend(value)
	}
}

// returns the delete first item
func (list *List[T]) PopFirst() (value T) {
	if len(*list) > 0 {
		value = (*list)[0]
		*list = (*list)[1:]
	}
	return
}

// returns the deleted last item
func (list *List[T]) PopLast() (value T) {
	if i := len(*list); i > 0 {
		value = (*list)[i-1]
		*list = (*list)[:i-1]
	}
	return
}

// returns index of the given item in the list
func (list List[T]) Index(value T) int {
	for i, v := range list {
		if v == value {
			return i
		}
	}
	return NA
}

// delete item at the given index from list
func (list *List[T]) DeleteIndex(index int) {
	if list == nil || index < 0 || index >= len(*list) {
		return
	}
	*list = append((*list)[:index], (*list)[index+1:]...)
}

// delete item with the given value from the list
func (list *List[T]) Delete(value T) {
	list.DeleteIndex(list.Index(value))
}
