<?php

namespace FlowCatalyst\Generated\Endpoint;

class PostApiBffEventTypesByIdSchemasByVersionFinalise extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $id;
    protected $version;
    /**
     * @param string $id
     * @param string $version
     */
    public function __construct(string $id, string $version)
    {
        $this->id = $id;
        $this->version = $version;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'POST';
    }
    public function getUri(): string
    {
        return str_replace(['{id}', '{version}'], [$this->id, $this->version], '/api/bff/event-types/{id}/schemas/{version}/finalise');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        return [[], null];
    }
    public function getExtraHeaders(): array
    {
        return ['Accept' => ['application/json']];
    }
    /**
     * {@inheritdoc}
     *
     * @throws \FlowCatalyst\Generated\Exception\PostApiBffEventTypesByIdSchemasByVersionFinaliseNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PostApiBffEventTypesByIdSchemasByVersionFinaliseConflictException
     *
     * @return null|\FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasVersionFinalisePostResponse200
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasVersionFinalisePostResponse200', 'json');
        }
        if (is_null($contentType) === false && (404 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\PostApiBffEventTypesByIdSchemasByVersionFinaliseNotFoundException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasVersionFinalisePostResponse404', 'json'), $response);
        }
        if (is_null($contentType) === false && (409 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\PostApiBffEventTypesByIdSchemasByVersionFinaliseConflictException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasVersionFinalisePostResponse409', 'json'), $response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return ['bearerAuth'];
    }
}