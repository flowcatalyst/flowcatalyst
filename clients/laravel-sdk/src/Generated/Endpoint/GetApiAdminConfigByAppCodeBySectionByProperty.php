<?php

namespace FlowCatalyst\Generated\Endpoint;

class GetApiAdminConfigByAppCodeBySectionByProperty extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $appCode;
    protected $section;
    protected $property;
    /**
     * @param string $appCode
     * @param string $section
     * @param string $property
     * @param array{
     *    "scope"?: string,
     *    "clientId"?: string,
     * } $queryParameters
     */
    public function __construct(string $appCode, string $section, string $property, array $queryParameters = [])
    {
        $this->appCode = $appCode;
        $this->section = $section;
        $this->property = $property;
        $this->queryParameters = $queryParameters;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'GET';
    }
    public function getUri(): string
    {
        return str_replace(['{appCode}', '{section}', '{property}'], [$this->appCode, $this->section, $this->property], '/api/admin/config/{appCode}/{section}/{property}');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        return [[], null];
    }
    public function getExtraHeaders(): array
    {
        return ['Accept' => ['application/json']];
    }
    protected function getQueryOptionsResolver(): \Symfony\Component\OptionsResolver\OptionsResolver
    {
        $optionsResolver = parent::getQueryOptionsResolver();
        $optionsResolver->setDefined(['scope', 'clientId']);
        $optionsResolver->setRequired([]);
        $optionsResolver->setDefaults([]);
        $optionsResolver->addAllowedTypes('scope', ['string']);
        $optionsResolver->addAllowedTypes('clientId', ['string']);
        return $optionsResolver;
    }
    /**
     * {@inheritdoc}
     *
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminConfigByAppCodeBySectionByPropertyForbiddenException
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminConfigByAppCodeBySectionByPropertyNotFoundException
     *
     * @return null|\FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyGetResponse200
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyGetResponse200', 'json');
        }
        if (is_null($contentType) === false && (403 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\GetApiAdminConfigByAppCodeBySectionByPropertyForbiddenException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyGetResponse403', 'json'), $response);
        }
        if (is_null($contentType) === false && (404 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\GetApiAdminConfigByAppCodeBySectionByPropertyNotFoundException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyGetResponse404', 'json'), $response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return ['bearerAuth'];
    }
}