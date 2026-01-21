package tech.flowcatalyst.event.panache;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.persistence.EntityManager;
import tech.flowcatalyst.event.Event;
import tech.flowcatalyst.event.EventRepository;
import tech.flowcatalyst.event.entity.EventEntity;
import tech.flowcatalyst.event.mapper.EventMapper;
import tech.flowcatalyst.platform.common.Page;

import java.util.List;
import java.util.Optional;

/**
 * Read-side repository for Event entities.
 * Uses EntityManager directly to return domain objects without conflicts.
 */
@ApplicationScoped
public class EventReadRepository implements EventRepository {

    @Inject
    EntityManager em;

    @Inject
    EventWriteRepository writeRepo;

    @Override
    public Event findById(String id) {
        return findByIdOptional(id).orElse(null);
    }

    @Override
    public Optional<Event> findByIdOptional(String id) {
        EventEntity entity = em.find(EventEntity.class, id);
        return Optional.ofNullable(entity).map(EventMapper::toDomain);
    }

    @Override
    public Optional<Event> findByDeduplicationId(String deduplicationId) {
        var results = em.createQuery(
                "FROM EventEntity WHERE deduplicationId = :deduplicationId", EventEntity.class)
            .setParameter("deduplicationId", deduplicationId)
            .getResultList();
        return results.isEmpty() ? Optional.empty() : Optional.of(EventMapper.toDomain(results.get(0)));
    }

    @Override
    public List<Event> listAll() {
        return em.createQuery("FROM EventEntity", EventEntity.class)
            .getResultList()
            .stream()
            .map(EventMapper::toDomain)
            .toList();
    }

    @Override
    public List<Event> findRecentPaged(int page, int size) {
        return em.createQuery("FROM EventEntity ORDER BY time DESC", EventEntity.class)
            .setFirstResult(page * size)
            .setMaxResults(size)
            .getResultList()
            .stream()
            .map(EventMapper::toDomain)
            .toList();
    }

    @Override
    @Deprecated
    public long count() {
        return em.createQuery("SELECT COUNT(e) FROM EventEntity e", Long.class)
            .getSingleResult();
    }

    @Override
    public boolean existsByDeduplicationId(String deduplicationId) {
        Long count = em.createQuery(
                "SELECT COUNT(e) FROM EventEntity e WHERE e.deduplicationId = :deduplicationId", Long.class)
            .setParameter("deduplicationId", deduplicationId)
            .getSingleResult();
        return count > 0;
    }

    @Override
    public Page<Event> findPage(String afterCursor, int limit) {
        List<EventEntity> entities;

        if (afterCursor == null || afterCursor.isBlank()) {
            entities = em.createQuery("FROM EventEntity ORDER BY id DESC", EventEntity.class)
                .setMaxResults(limit + 1)
                .getResultList();
        } else {
            entities = em.createQuery("FROM EventEntity WHERE id < :cursor ORDER BY id DESC", EventEntity.class)
                .setParameter("cursor", afterCursor)
                .setMaxResults(limit + 1)
                .getResultList();
        }

        List<Event> events = entities.stream()
            .map(EventMapper::toDomain)
            .toList();

        return Page.of(events, limit, Event::id);
    }

    // Write operations delegate to WriteRepository
    @Override
    public void insert(Event event) {
        writeRepo.persistEvent(event);
    }

    @Override
    public void persist(Event event) {
        writeRepo.persistEvent(event);
    }

    @Override
    public void persistAll(List<Event> events) {
        writeRepo.persistAllEvents(events);
    }

    @Override
    public void update(Event event) {
        writeRepo.updateEvent(event);
    }

    @Override
    public void delete(Event event) {
        writeRepo.deleteEventById(event.id);
    }

    @Override
    public boolean deleteById(String id) {
        return writeRepo.deleteEventById(id);
    }
}
