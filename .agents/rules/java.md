---
paths:
  - "**/*.java"
---

# Java

## Versao do Java

Utilize Java 21+ como versao minima. Aproveite as features modernas da linguagem.

**Exemplo:**
```java
// ✅ Prefira - features modernas
var users = List.of("Alice", "Bob");
record UserDTO(String name, String email) {}

// ❌ Evite - estilo legado
List<String> users = new ArrayList<>();
users.add("Alice");
users.add("Bob");
```

## Records

Prefira `record` para DTOs e value objects imutaveis. Evite classes com getters/setters manuais para dados imutaveis.

**Exemplo:**
```java
// ❌ Evite
public class UserDTO {
  private String name;
  private String email;

  public UserDTO(String name, String email) {
    this.name = name;
    this.email = email;
  }

  public String getName() { return name; }
  public String getEmail() { return email; }
}

// ✅ Prefira
public record UserDTO(String name, String email) {}

// Com validacao
public record UserDTO(String name, String email) {
  public UserDTO {
    if (name == null || name.isBlank()) {
      throw new IllegalArgumentException("Nome nao pode ser vazio");
    }
  }
}
```

## Inferencia de Tipo com var

Use `var` para variaveis locais quando o tipo e obvio pelo contexto. Evite `var` quando o tipo nao e claro.

**Exemplo:**
```java
// ✅ Prefira - tipo obvio pelo contexto
var users = new ArrayList<User>();
var name = "John";
var count = users.size();
var response = httpClient.send(request, BodyHandlers.ofString());

// ❌ Evite - tipo nao e claro
var result = process(data); // que tipo e result?
var x = getValue(); // ambiguo
```

## Streams e Colecoes

Prefira Stream API (`filter`, `map`, `collect`) ao inves de loops imperativos. Use `List.of()`, `Map.of()` para colecoes imutaveis.

**Exemplo:**
```java
// ❌ Evite
List<String> names = new ArrayList<>();
for (User user : users) {
  if (user.age() > 25) {
    names.add(user.name());
  }
}

// ✅ Prefira
var names = users.stream()
    .filter(user -> user.age() > 25)
    .map(User::name)
    .toList();

// Colecoes imutaveis
var colors = List.of("red", "green", "blue");
var config = Map.of("key1", "value1", "key2", "value2");
```

## Optional

Retorne `Optional<T>` ao inves de `null`. Nunca passe `Optional` como parametro de metodo. Use `orElseThrow()`, `map()`, `ifPresent()`.

**Exemplo:**
```java
// ❌ Evite
public User findUser(String id) {
  User user = repository.find(id);
  return user; // pode retornar null
}

// ❌ Evite
public void process(Optional<User> user) { } // Optional como parametro

// ❌ Evite
Optional<User> opt = findUser("123");
if (opt.isPresent()) {
  User user = opt.get(); // anti-pattern
}

// ✅ Prefira
public Optional<User> findUser(String id) {
  return repository.findById(id);
}

// Uso
var user = findUser("123").orElseThrow(() -> new UserNotFoundException("123"));
var name = findUser("123").map(User::name).orElse("Desconhecido");
findUser("123").ifPresent(u -> sendEmail(u.email()));
```

## Sealed Classes

Use `sealed` + `permits` para hierarquias fechadas de tipos.

**Exemplo:**
```java
// ✅ Prefira
public sealed interface PaymentMethod permits CreditCard, Pix, Boleto {
  String description();
}

public record CreditCard(String number, String holder) implements PaymentMethod {
  public String description() { return "Cartao " + number.substring(number.length() - 4); }
}

public record Pix(String key) implements PaymentMethod {
  public String description() { return "PIX " + key; }
}

public record Boleto(String code) implements PaymentMethod {
  public String description() { return "Boleto " + code; }
}
```

## Text Blocks

Use text blocks (`"""`) para strings multilinhas (SQL, JSON, mensagens).

**Exemplo:**
```java
// ❌ Evite
String sql = "SELECT u.id, u.name, u.email " +
    "FROM users u " +
    "WHERE u.active = true " +
    "ORDER BY u.name";

// ✅ Prefira
var sql = """
    SELECT u.id, u.name, u.email
    FROM users u
    WHERE u.active = true
    ORDER BY u.name
    """;
```

## Pattern Matching

Use `instanceof` com pattern matching para evitar casts explicitos.

**Exemplo:**
```java
// ❌ Evite
if (obj instanceof String) {
  String s = (String) obj;
  System.out.println(s.length());
}

// ✅ Prefira
if (obj instanceof String s) {
  System.out.println(s.length());
}

// Com switch
switch (shape) {
  case Circle c -> System.out.println("Circulo raio: " + c.radius());
  case Rectangle r -> System.out.println("Retangulo: " + r.width() + "x" + r.height());
  default -> System.out.println("Forma desconhecida");
}
```

## Imutabilidade

Prefira objetos imutaveis. Use `final` em campos e variaveis locais quando possivel. Use `List.copyOf()` para copias imutaveis.

**Exemplo:**
```java
// ✅ Prefira
public class UserService {
  private final UserRepository repository;
  private final EmailService emailService;

  public UserService(UserRepository repository, EmailService emailService) {
    this.repository = repository;
    this.emailService = emailService;
  }
}

// Copias defensivas
public List<User> getUsers() {
  return List.copyOf(this.users);
}
```

## Nomenclatura

- Classes e interfaces: `PascalCase` (`UserService`, `PaymentMethod`)
- Metodos e variaveis: `camelCase` (`findUser`, `userName`)
- Constantes: `UPPER_SNAKE_CASE` (`MAX_RETRY_COUNT`, `DEFAULT_TIMEOUT`)
- Pacotes: `lowercase` (`com.empresa.pedidos`)

**Exemplo:**
```java
// ✅ Prefira
public class OrderService {
  private static final int MAX_ITEMS_PER_ORDER = 100;

  public OrderDTO createOrder(CreateOrderRequest request) {
    // implementacao
  }
}
```

## Tratamento de Excecoes

Nunca capture `Exception` ou `Throwable` genericamente. Crie excecoes de dominio especificas. Nunca ignore excecoes silenciosamente.

**Exemplo:**
```java
// ❌ Evite
try {
  processOrder(order);
} catch (Exception e) {
  // ignora silenciosamente
}

// ❌ Evite
try {
  processOrder(order);
} catch (Exception e) {
  e.printStackTrace(); // nunca em producao
}

// ✅ Prefira
public class OrderNotFoundException extends RuntimeException {
  public OrderNotFoundException(String orderId) {
    super("Pedido nao encontrado: " + orderId);
  }
}

try {
  processOrder(order);
} catch (OrderNotFoundException e) {
  log.warn("Pedido nao encontrado", e);
  throw e;
} catch (PaymentException e) {
  log.error("Erro no pagamento do pedido {}", order.id(), e);
  throw new OrderProcessingException("Falha ao processar pagamento", e);
}
```

## Anotacoes de Nulidade

Use `@NonNull`/`@Nullable` para documentar contratos de nulidade.

**Exemplo:**
```java
import org.springframework.lang.NonNull;
import org.springframework.lang.Nullable;

public class UserService {
  @NonNull
  public User getUser(@NonNull String id) {
    return repository.findById(id)
        .orElseThrow(() -> new UserNotFoundException(id));
  }

  @Nullable
  public User findUserByEmail(@NonNull String email) {
    return repository.findByEmail(email).orElse(null);
  }
}
```
