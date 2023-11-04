default:
	@echo "Check targets"

crep:
	$(CC) main.c -o crep

clean:
	rm crep
