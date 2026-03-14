<?php

namespace FlowCatalyst\Generated\Endpoint;

class GetApiAdminClientsByIdentifierByIdentifier extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $identifier;
    /**
     * @param string $identifier
     */
    public function __construct(string $identifier)
    {
        $this->identifier = $identifier;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'GET';
    }
    public function getUri(): string
    {
        return str_replace(['{identifier}'], [$this->identifier], '/api/admin/clients/by-identifier/{identifier}');
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
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminClientsByIdentifierByIdentifierNotFoundException
     *
     * @return null|\FlowCatalyst\Generated\Model\ApiAdminClientsByIdentifierIdentifierGetResponse200
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminClientsByIdentifierIdentifierGetResponse200', 'json');
        }
        if (is_null($contentType) === false && (404 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\GetApiAdminClientsByIdentifierByIdentifierNotFoundException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminClientsByIdentifierIdentifierGetResponse404', 'json'), $response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return ['bearerAuth'];
    }
}